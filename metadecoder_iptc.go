package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
)

func newMetaDecoderIPTC(r io.Reader, callback HandleTagFunc) *metaDecoderIPTC {
	return &metaDecoderIPTC{
		streamReader: newStreamReader(r),
		handleTag:    callback,
	}
}

type metaDecoderIPTC struct {
	*streamReader
	handleTag HandleTagFunc
}

func (e *metaDecoderIPTC) decode() (err error) {
	// Skip the IPTC header.
	e.skip(14)

	const iptcMetaDataBlockID = 0x0404

	stringSlices := make(map[uint8][]string)

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

			e.skip(1) // recordType
			datasetNumber := e.read1()
			recordSize := e.read2()

			recordDef, ok := iptcFieldMap[datasetNumber]
			if !ok {
				// Assume a non repeatable string.
				recordDef = iptcField{
					name:       fmt.Sprintf("%s%d", UnknownPrefix, datasetNumber),
					format:     "string",
					repeatable: false,
				}
			}

			var v any
			switch recordDef.format {
			case "string":
				b := e.readBytesVolatile(int(recordSize))
				v = string(b)
			case "short":
				v = e.read2()
			case "byte":
				v = e.read1()
			default:
				panic(fmt.Sprintf("unhandled format %q", recordDef.format))
			}

			if recordDef.repeatable {
				stringSlices[datasetNumber] = append(stringSlices[datasetNumber], v.(string))
			} else {
				if err := e.handleTag(TagInfo{
					Source: TagSourceIPTC,
					Tag:    recordDef.name,
					Value:  v,
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
		for datasetNumber, values := range stringSlices {
			if err := e.handleTag(TagInfo{
				Source: TagSourceIPTC,
				Tag:    iptcFieldMap[datasetNumber].name,
				Value:  values,
			}); err != nil {
				return err
			}
		}

	}

	return nil

}

type iptcField struct {
	name       string
	repeatable bool
	format     string
}

var iptcFieldMap = map[uint8]iptcField{
	0:   {"RecordVersion", false, "short"},
	4:   {"ObjectTypeReference", false, "string"},
	5:   {"ObjectName", false, "string"},
	7:   {"EditStatus", false, "string"},
	10:  {"Urgency", false, "byte"},
	12:  {"SubjectReference", true, "string"},
	15:  {"Category", true, "string"},
	20:  {"SupplementalCategory", true, "string"},
	22:  {"FixtureIdentifier", false, "string"},
	25:  {"Keywords", true, "string"},
	26:  {"ContentLocationCode", false, "string"},
	27:  {"ContentLocationName", false, "string"},
	30:  {"ReleaseDate", false, "string"},
	35:  {"ReleaseTime", false, "string"},
	37:  {"ExpirationDate", false, "string"},
	38:  {"ExpirationTime", false, "string"},
	40:  {"SpecialInstructions", false, "string"},
	42:  {"ActionAdvised", false, "B"},
	45:  {"ReferenceService", false, "string"},
	47:  {"ReferenceDate", false, "string"},
	50:  {"ReferenceNumber", false, "string"},
	55:  {"DateCreated", false, "string"},
	60:  {"TimeCreated", false, "string"},
	62:  {"DigitalCreationDate", false, "string"},
	63:  {"DigitalCreationTime", false, "string"},
	65:  {"OriginatingProgram", false, "string"},
	70:  {"ProgramVersion", false, "string"},
	75:  {"ObjectCycle", false, "string"},
	80:  {"Byline", false, "string"},
	85:  {"BylineTitle", false, "string"},
	90:  {"City", false, "string"},
	92:  {"SubLocation", false, "string"},
	95:  {"ProvinceState", false, "string"},
	100: {"CountryCode", false, "string"},
	101: {"CountryName", false, "string"},
	103: {"OriginalTransmissionReference", false, "string"},
	105: {"Headline", false, "string"},
	110: {"Credit", false, "string"},
	115: {"Source", false, "string"},
	116: {"Copyright", false, "string"},
	118: {"Contact", false, "string"},
	120: {"Caption", false, "string"},
	122: {"LocalCaption", false, "string"},
}
