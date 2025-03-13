// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	_ "embed" // needed for the embedded IPTC fields JSON
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// Source: https://exiftool.org/TagNames/IPTC.html
//
//go:embed metadecoder_iptc_fields.json
var ipctTagsJSON []byte

var (
	iptcRecordFields = map[uint8]map[uint8]iptcField{}
	iptcRerordNames  = map[uint8]string{
		1:   "IPTCEnvelope",
		2:   "IPTCApplication",
		3:   "IPTCNewsPhoto",
		7:   "IPTCPreObjectData",
		8:   "IPTCObjectData",
		9:   "IPTCPostObjectData",
		240: "IPTCFotoStation",
	}
)

const (
	ipcCodedCharacterSet = 90
	iptcMetaDataBlockID  = 0x0404
)

type vcIPTC struct {
	//*vc
}

func (c *vcIPTC) convertDateString(ctx valueConverterContext, v any) any {
	s := toString(v)
	// 20211020 => 2021:10:20
	if len(s) == 8 {
		return fmt.Sprintf("%s:%s:%s", s[:4], s[4:6], s[6:])
	}
	// 2015-01-22 => 2015:01:22
	if len(s) == 10 {
		return fmt.Sprintf("%s:%s:%s", s[:4], s[5:7], s[8:])
	}
	return s
}

func (c *vcIPTC) convertTime(ctx valueConverterContext, v any) any {
	s := toString(v)
	// 111116 => 11:11:16
	if len(s) == 6 {
		return fmt.Sprintf("%s:%s:%s", s[:2], s[2:4], s[4:])
	}
	// 130444+1000 => 13:04:44+10:00
	if len(s) == 11 {
		return fmt.Sprintf("%s:%s:%s%s:%s", s[:2], s[2:4], s[4:6], s[6:9], s[9:])
	}
	return s
}

var (
	iptcConverters        = &vcIPTC{}
	iptcValueConverterMap = map[string]valueConverter{
		"DateCreated":         iptcConverters.convertDateString,
		"DateSent":            iptcConverters.convertDateString,
		"DigitalCreationDate": iptcConverters.convertDateString,
		"DigitalCreationTime": iptcConverters.convertTime,
		"TimeSent":            iptcConverters.convertTime,
		"TimeCreated": func(ctx valueConverterContext, v any) any {
			s := toString(v)
			if len(s) == 11 {
				// 210101+0000 => 21:01:01+00:00
				return fmt.Sprintf("%s:%s:%s%s:%s", s[:2], s[2:4], s[4:7], s[7:9], s[9:])
			}
			if len(s) == 6 {
				// 124633 => 12:46:33
				return fmt.Sprintf("%s:%s:%s", s[:2], s[2:4], s[4:])
			}
			return s
		},
		"ProgramVersion": func(ctx valueConverterContext, v any) any {
			s := toString(v)
			s = strings.TrimSuffix(s, ".0")
			return s
		},
		"CodedCharacterSet": func(ctx valueConverterContext, v any) any {
			b := v.([]byte)
			s := resolveCodedCharacterSet(b)
			if s == "" {
				return characterSetUTF8
			}
			return s
		},
	}
)

func newMetaDecoderIPTC(r io.Reader, opts Options) *metaDecoderIPTC {
	s := newStreamReader(r, binary.BigEndian)
	return &metaDecoderIPTC{
		streamReader:           s,
		iso88591CharsetDecoder: charmap.ISO8859_1.NewDecoder(),
		valueConverterContext: valueConverterContext{
			s:         s,
			warnfFunc: opts.Warnf,
		},
		opts: opts,
	}
}

type iptcField struct {
	Record     uint8  `json:"record"`
	RecordName string `json:"record_name"`
	ID         uint8  `json:"id"`
	Name       string `json:"name"`
	Format     string `json:"format"`
	Repeatable bool   `json:"repeatable"`
	Notes      string `json:"notes"`
}

type metaDecoderIPTC struct {
	*streamReader

	charset                string
	iso88591CharsetDecoder *encoding.Decoder
	valueConverterContext  valueConverterContext

	opts Options
}

// Decode decodes the IPTC records delimited by 0x1C.
func (e *metaDecoderIPTC) decodeRecords() (err error) {
	stringSlices := make(map[TagInfo][]string)
	for {
		var marker uint8
		if err := binary.Read(e.r, e.byteOrder, &marker); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if marker != 0x1C {
			break
		}

		if err := e.decodeRecord(stringSlices); err != nil {
			return err
		}
	}

	if err := e.handlestringSlices(stringSlices); err != nil {
		return err
	}

	return nil
}

func (e *metaDecoderIPTC) handlestringSlices(m map[TagInfo][]string) error {
	if len(m) == 0 {
		return nil
	}
	for ti, values := range m {
		if len(values) == 0 || len(values) > 1 {
			ti.Value = values
		} else {
			ti.Value = values[0]
		}
		if err := e.opts.HandleTag(ti); err != nil {
			return err
		}
	}
	return nil
}

// decodeBlocks decodes the IPTC data from segments separated by 8BIM.
// This assumes a reader that starts out at 8BIM (no headers)
func (e *metaDecoderIPTC) decodeBlocks() (err error) {
	stringSlices := make(map[TagInfo][]string)

	decodeBlock := func() error {
		blockType := e.readBytesVolatile(4)

		if string(blockType) != "8BIM" {
			return errStop
		}

		identifier := e.read2()
		isNotMeta := identifier != iptcMetaDataBlockID

		nameLength := e.read1()
		if nameLength == 0 {
			nameLength = 2
		} else if nameLength%2 == 1 {
			nameLength++
		}

		e.skip(int64(nameLength - 1))

		dataSize := e.read4()

		if isNotMeta {
			e.skip(int64(dataSize))
			return nil
		}

		if dataSize%2 != 0 {
			defer func() {
				// Skip padding byte.
				e.skip(1)
			}()
		}

		for {
			marker := e.read1()

			if e.isEOF || marker != 0x1C {
				return errStop
			}

			if err := e.decodeRecord(stringSlices); err != nil {
				return err
			}
		}
	}

	for {
		if err := decodeBlock(); err != nil {
			if err == errStop {
				break
			}
			return err
		}
	}

	if err := e.handlestringSlices(stringSlices); err != nil {
		return err
	}

	return nil
}

func (e *metaDecoderIPTC) decodeRecord(stringSlices map[TagInfo][]string) error {
	recordType := e.read1()
	datasetNumber := e.read1()
	recordSize := e.read2()

	recordDef, ok := getIptcRecordFieldDef(recordType, datasetNumber)

	if !ok {
		// Assume a non repeatable string.
		recordDef = iptcField{
			Name:       fmt.Sprintf("%s%d", UnknownPrefix, datasetNumber),
			RecordName: "IPTCUnknownRecord",
			Format:     "string",
			Repeatable: false,
		}
	}

	ti := TagInfo{
		Source:    IPTC,
		Tag:       recordDef.Name,
		Namespace: recordDef.RecordName,
	}

	if recordSize > uint16(e.opts.LimitTagSize) || !e.opts.ShouldHandleTag(ti) {
		e.skip(int64(recordSize))
		return nil
	}

	var v any
	switch recordDef.Format {
	case "string":
		v = e.readBytesVolatile(int(recordSize))
		if e.charset == "" || e.charset == characterSetISO88591 {
			v, _ = e.iso88591CharsetDecoder.Bytes(v.([]byte))
		}
	case "uint32":
		v = e.read4()
	case "short":
		v = e.read2()
	case "byte":
		v = e.read1()
	default:
		panic(fmt.Errorf("unsupported format %q", recordDef.Format))
	}

	if convert, found := iptcValueConverterMap[recordDef.Name]; found {
		e.valueConverterContext.tagName = recordDef.Name
		v = convert(e.valueConverterContext, v)
	}

	if recordType == 1 && datasetNumber == ipcCodedCharacterSet {
		e.charset = v.(string)
	}

	if b, ok := v.([]byte); ok {
		v = strings.TrimSpace(string(trimBytesNulls(b)))
	}

	if recordDef.Repeatable {
		stringSlices[ti] = append(stringSlices[ti], toString(v))
	} else {
		ti.Value = v
		if err := e.opts.HandleTag(ti); err != nil {
			return err
		}
	}

	return nil
}

func getIptcRecordFieldDef(record, id uint8) (iptcField, bool) {
	recordFields, ok := iptcRecordFields[record]
	if !ok {
		return iptcField{}, false
	}
	field, ok := recordFields[id]
	return field, ok
}

func getIptcRecordName(record uint8) string {
	name, ok := iptcRerordNames[record]
	if !ok {
		return fmt.Sprintf("IPTCUnknownRecord%d", record)
	}
	return name
}

func init() {
	var fields []map[string]any
	if err := json.Unmarshal(ipctTagsJSON, &fields); err != nil {
		panic(err)
	}

	toUint8 := func(v any) uint8 {
		s := v.(string)
		i, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		return uint8(i)
	}

	toString := func(v any) string {
		if v == nil {
			return ""
		}
		return v.(string)
	}

	for _, fieldv := range fields {
		id := toUint8(fieldv["id"])
		record := toUint8(fieldv["record"])
		recordFields, ok := iptcRecordFields[record]
		if !ok {
			recordFields = map[uint8]iptcField{}
			iptcRecordFields[record] = recordFields
		}

		recordFields[id] = iptcField{
			Record:     record,
			RecordName: getIptcRecordName(record),
			ID:         id,
			Name:       toString(fieldv["name"]),
			Format:     toString(fieldv["format"]),
			Notes:      toString(fieldv["notes"]),
			Repeatable: fieldv["repeatable"] == "true",
		}
	}
}

const (
	characterSetUTF8     = "UTF-8"
	characterSetISO88591 = "ISO-8859-1"
)

// resolveCodedCharacterSet resolves the coded character set from the IPTC data
// to be either UTF-8 or ISO-8859-1 or an empty string if it cannot be resolved.
func resolveCodedCharacterSet(b []byte) string {
	const (
		esc           = 0x1B
		percent       = 0x25
		latinCapitalG = 0x47
		dot           = 0x2E
		latinCapitalA = 0x41
		minus         = 0x2D
	)

	if len(b) > 2 && b[0] == esc && b[1] == percent && b[2] == latinCapitalG {
		return characterSetUTF8
	}

	if len(b) > 2 && b[0] == esc && b[1] == dot && b[2] == latinCapitalA {
		return characterSetISO88591
	}

	if len(b) > 3 && b[0] == esc && (b[1] == dot || b[2] == dot || b[3] == dot) && b[4] == latinCapitalA {
		return characterSetISO88591
	}

	if len(b) > 2 && b[0] == esc && b[1] == minus && b[2] == latinCapitalA {
		return characterSetISO88591
	}

	return ""
}
