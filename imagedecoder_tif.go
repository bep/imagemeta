package imagemeta

import (
	"encoding/binary"
	"io"
)

type imageDecoderTIF struct {
	*baseStreamingDecoder
}

func (e *imageDecoderTIF) decode() error {
	const (
		xmpMarker     = 0x02bc
		meaningOfLife = 42
	)

	// These are the sources we currently support in TIFF.
	sourceSet := XMP
	// Remove sources that are not requested.
	sourceSet = sourceSet & e.opts.Sources

	if sourceSet.IsZero() {
		// Done.
		return nil
	}

	byteOrderTag := e.read2()
	switch byteOrderTag {
	case byteOrderBigEndian:
		e.byteOrder = binary.BigEndian
	case byteOrderLittleEndian:
		e.byteOrder = binary.LittleEndian
	default:
		return errInvalidFormat
	}

	if id := e.read2(); id != meaningOfLife {
		return errInvalidFormat
	}

	ifdOffset := e.read4()

	if ifdOffset < 8 {
		return errInvalidFormat
	}

	e.skip(int64(ifdOffset - 8))

	entryCount := e.read2()

	for i := 0; i < int(entryCount); i++ {
		tag := e.read2()
		// Skip type
		e.skip(2)
		count := e.read4()
		valueOffset := e.read4() // Offset relative to the start of the file.
		if tag == xmpMarker {
			pos := e.pos()
			e.seek(int(valueOffset))
			r := io.LimitReader(e.r, int64(count))
			if err := decodeXMP(r, e.opts); err != nil {
				return err
			}
			sourceSet = sourceSet.Remove(XMP)
			if sourceSet.IsZero() {
				return nil
			}
			e.seek(pos)
		}
	}

	return nil
}
