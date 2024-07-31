// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/binary"
)

type imageDecoderTIF struct {
	*baseStreamingDecoder
}

func (e *imageDecoderTIF) decode() error {
	const (
		meaningOfLife = 42
	)

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

	dec := newMetaDecoderEXIFFromStreamReader(e.streamReader, 0, e.opts)

	return dec.decodeTags("IFD0")
}
