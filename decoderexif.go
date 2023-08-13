package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
)

func newDecoderEXIF(r io.Reader, callback HandleTagFunc) *decoderEXIF {
	return &decoderEXIF{
		streamReader: newStreamReader(r),
		handleTag:    callback,
	}
}

type decoderEXIF struct {
	*streamReader
	handleTag HandleTagFunc
}

func (e *decoderEXIF) convertValues(typ exifType, count, len int, r io.Reader) any {
	if count == 0 {
		return nil
	}

	if typ == typeUnsignedASCII {
		buff := getBuffer()
		defer putBuffer(buff)
		// Read len bytes into buff from r.
		n, err := io.CopyN(buff, r, int64(len))
		if err != nil || n != int64(len) {
			// TODO1
			panic(err)
		}
		return string(buff.Bytes()[:count-1])
	}

	if count == 1 {
		return e.convertValue(typ, r)
	}

	values := make([]any, count)
	for i := 0; i < count; i++ {
		values[i] = e.convertValue(typ, r)
	}
	return values
}

func (e *decoderEXIF) convertValue(typ exifType, r io.Reader) any {
	switch typ {
	case typeUnsignedByte, typeUndef:
		return e.read1r(r)
	case typeUnsignedShort:
		return e.read2r(r)
	case typeUnsignedLong:
		return e.read4r(r)
	case typeSignedLong:
		return e.read4Signedr(r)
	case typeUnsignedRat:
		n, d := e.read4r(r), e.read4r(r)
		return big.NewRat(int64(n), int64(d))
	case typeSignedRat:
		n, d := e.read4Signedr(r), e.read4Signedr(r)
		return big.NewRat(int64(n), int64(d))
	default:
		// TODO1
		panic(fmt.Errorf("exif type %d not implemented", typ))
	}
}

func (e *decoderEXIF) decode() (err error) {
	byteOrderTag := e.read2()

	switch byteOrderTag {
	case byteOrderBigEndian:
		e.byteOrder = binary.BigEndian
	case byteOrderLittleEndian:
		e.byteOrder = binary.LittleEndian
	default:

		return nil
	}

	e.skip(2)

	firstOffset := e.read4()

	if firstOffset < 8 {
		return nil
	}

	e.skip(int64(firstOffset - 8))
	e.readerOffset = e.pos() - 8

	return e.decodeTags()

}

func (e *decoderEXIF) decodeTags() error {
	if e.done() {
		e.stop(nil)
	}

	numTags := e.read2()

	for i := 0; i < int(numTags); i++ {
		if err := e.decodeTag(); err != nil {
			return err
		}
	}

	return nil
}

func (e *decoderEXIF) decodeTagsAT(offset int) error {
	oldPos := e.pos()
	defer func() {
		e.seek(oldPos)
	}()
	e.seek(offset + e.readerOffset)
	return e.decodeTags()
}

func (e *decoderEXIF) done() bool {
	return false // TODO1
}
