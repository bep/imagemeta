package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
)

func newDecoderIPTC(r io.Reader, callback HandleTagFunc) *decoderIPTC {
	return &decoderIPTC{
		streamReader: newStreamReader(r),
		handleTag:    callback,
	}
}

type decoderIPTC struct {
	*streamReader
	handleTag HandleTagFunc
}

func (e *decoderIPTC) decode() (err error) {
	// Skip the IPTC header.
	e.skip(14)

	const iptcMetaDataBlockID = 0x0404

	decodeBlock := func() error {
		blockType := make([]byte, 4)
		e.readFull(blockType)
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

			var recordType, datasetNumber uint8
			var recordSize uint16
			if err := binary.Read(r, e.byteOrder, &recordType); err != nil {
				return err
			}
			if err := binary.Read(r, e.byteOrder, &datasetNumber); err != nil {
				return err
			}
			if err := binary.Read(r, e.byteOrder, &recordSize); err != nil {
				return err
			}

			recordBytes := make([]byte, recordSize)
			if err := binary.Read(r, e.byteOrder, recordBytes); err != nil {
				return err
			}

			// TODO1 get an up to date field map.
			// TODO1 handle unkonwn dataset numbers.
			recordDef, ok := iptcFieldMap[datasetNumber]
			if !ok {
				fmt.Println("unknown datasetNumber", datasetNumber)
				continue
			}

			var v any
			switch recordDef.format {
			case "string":
				v = string(recordBytes)
			case "B": // TODO1 check these
				v = recordBytes
			}

			if err := e.handleTag(TagInfo{
				Source: TagSourceIPTC,
				Tag:    recordDef.name,
				Value:  v,
			}); err != nil {
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

	return nil

}
