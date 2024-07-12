package imagemeta

import (
	_ "embed" // needed for the embedded IPTC fields JSON
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
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

func newMetaDecoderIPTC(r io.Reader, opts Options) *metaDecoderIPTC {
	return &metaDecoderIPTC{
		streamReader: newStreamReader(r),
		opts:         opts,
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
	opts Options
}

func (e *metaDecoderIPTC) decode() (err error) {
	// Skip the IPTC header.
	e.skip(14)

	const iptcMetaDataBlockID = 0x0404

	stringSlices := make(map[iptcField][]string)

	decodeBlock := func() error {
		blockType := e.readBytesVolatile(4)
		if string(blockType) != "8BIM" {
			return errStop
		}

		identifier := e.read2()
		isMeta := identifier == iptcMetaDataBlockID

		nameLength := e.read1()
		if nameLength == 0 {
			nameLength = 2
		} else if nameLength%2 == 1 {
			nameLength++
		}

		e.skip(int64(nameLength - 1))
		dataSize := e.read4()

		if !isMeta {
			e.skip(int64(dataSize))
			return nil
		}

		// TODO1 extended datasets.

		if dataSize%2 != 0 {
			defer func() {
				// Skip padding byte.
				e.skip(1)
			}()
		}

		r := io.LimitReader(e.r, int64(dataSize))

		for {
			var marker uint8
			if err := binary.Read(r, e.byteOrder, &marker); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			if marker != 0x1C {
				return errStop
			}

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

			var v any
			switch recordDef.Format {
			case "string":
				b := e.readBytesVolatile(int(recordSize))
				v = string(b)
				// TODO1 validate these against record size.
			case "short":
				v = e.read2()
			case "byte":
				v = e.read1()
			default:
				panic(fmt.Sprintf("unhandled format %q", recordDef.Format))
			}

			if recordDef.Repeatable {
				stringSlices[recordDef] = append(stringSlices[recordDef], v.(string))
			} else {
				if err := e.opts.HandleTag(TagInfo{
					Source:    IPTC,
					Tag:       recordDef.Name,
					Namespace: recordDef.RecordName,
					Value:     v,
				}); err != nil {
					return err
				}
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

	if len(stringSlices) > 0 {
		for fieldDef, values := range stringSlices {
			if err := e.opts.HandleTag(
				TagInfo{
					Source:    IPTC,
					Tag:       fieldDef.Name,
					Namespace: fieldDef.RecordName,
					Value:     values,
				},
			); err != nil {
				return err
			}
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
	var fields []map[string]interface{}
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
