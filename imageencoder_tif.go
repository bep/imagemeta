// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/binary"
	"errors"
	"math"
	"slices"
)

type imageEncoderTIF struct {
	*baseEncoder
}

// encode appends a new IFD0 with the existing entries and the metadata
// to the end of the file and points the header to it.
func (e *imageEncoderTIF) encode() error {
	byteOrderTag := e.read2()
	switch byteOrderTag {
	case tiffMarker.byteOrderBE:
		e.byteOrder = binary.BigEndian
	case tiffMarker.byteOrderLE:
		e.byteOrder = binary.LittleEndian
	default:
		return newInvalidFormatErrorf("tiff: invalid byte order")
	}

	if id := e.read2(); e.err != nil || id != tiffMarker.meaningOfLife {
		return newInvalidFormatErrorf("tiff: invalid magic number")
	}

	ifdOffset := uint64(e.read4())
	if e.err != nil || ifdOffset < 8 || ifdOffset > uint64(len(e.b)) {
		return newInvalidFormatErrorf("tiff: invalid IFD0 offset")
	}
	e.seek(int(ifdOffset))

	numTags := e.read2()
	if e.err != nil || 12*uint64(numTags)+4 > uint64(e.remaining()) {
		return newInvalidFormatErrorf("tiff: invalid IFD0 entry count")
	}

	ifd0, exif, gps := e.meta.exifEntries(e.byteOrder)
	xmp := e.meta.encodeXMP()
	ifd0 = append(ifd0, ifdEntry{tag: tiffMarker.xmp, typ: exifTypeUnsignedByte1, count: uint32(len(xmp)), value: xmp})

	conflicts := []uint16{
		exifTagImageDescription,
		exifTagArtist,
		exifTagCopyright,
		tiffMarker.xmp,
		exifTagExifIFDPointer,
		exifTagGPSIFDPointer,
	}

	for range int(numTags) {
		tagID := e.read2()
		dataType := exifType(e.read2())
		count := e.read4()
		raw := e.readBytes(4)
		if slices.Contains(conflicts, tagID) {
			return errMetadataExistsf("tiff: tag 0x%04x in IFD0", tagID)
		}
		ifd0 = append(ifd0, ifdEntry{tag: tagID, typ: dataType, count: count, raw: raw})
	}
	next := e.read4()
	if e.err != nil {
		return e.err
	}

	totalIFD0 := len(ifd0)
	if len(exif) > 0 {
		totalIFD0++
	}
	if len(gps) > 0 {
		totalIFD0++
	}
	if totalIFD0 > math.MaxUint16 {
		return errors.New("tiff: too many IFD0 entries")
	}

	start := (len(e.b) + 1) &^ 1
	if start > math.MaxUint32/2 {
		return errors.New("tiff: file too large")
	}

	ifds := encodeIFDs(nil, e.byteOrder, uint32(start), ifd0, exif, gps, next)
	var hdr [8]byte
	copy(hdr[:4], e.b[:4])
	e.byteOrder.PutUint32(hdr[4:], uint32(start))

	var pad []byte
	if len(e.b)%2 == 1 {
		pad = []byte{0}
	}
	return e.writeChunks(hdr[:], e.b[8:], pad, ifds)
}
