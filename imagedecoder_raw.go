// Copyright 2026 Toni Melisma
// SPDX-License-Identifier: MIT

package imagemeta

import "encoding/binary"

const (
	rawMeaningOfLife      = 42
	rawTagImageWidth      = 0x0100
	rawTagImageHeight     = 0x0101
	rawTagSubIFDs         = 0x014a
	rawTagExifIFDPointer  = 0x8769
	rawTagExifImageWidth  = 0xa002
	rawTagExifImageHeight = 0xa003
	rawTagDefaultCropSize = 0xc620
)

type imageDecoderRAW struct {
	*baseStreamingDecoder
}

func (e *imageDecoderRAW) decode() error {
	byteOrderTag := e.read2()
	switch byteOrderTag {
	case byteOrderBigEndian:
		e.byteOrder = binary.BigEndian
	case byteOrderLittleEndian:
		e.byteOrder = binary.LittleEndian
	default:
		return errInvalidFormat
	}

	if id := e.read2(); id != rawMeaningOfLife {
		return errInvalidFormat
	}

	ifdOffset := e.read4()

	if ifdOffset < 8 {
		return errInvalidFormat
	}

	e.skip(int64(ifdOffset - 8))

	if e.opts.Sources.Has(CONFIG) {
		ifdPos := e.pos()

		var width, height int
		var exifIFDOffset uint32
		var subIFDOffsets []uint32
		var defaultCropW, defaultCropH int
		var hasDefaultCrop bool

		numTags := e.read2()
		for range int(numTags) {
			tagID := e.read2()
			dataType := e.read2()
			count := e.read4()

			switch tagID {
			case rawTagImageWidth, rawTagImageHeight:
				var value int
				if dataType == 3 { // SHORT
					value = int(e.read2())
					e.skip(2)
				} else { // LONG
					value = int(e.read4())
				}
				if tagID == rawTagImageWidth {
					width = value
				} else {
					height = value
				}
			case rawTagExifIFDPointer:
				exifIFDOffset = e.read4()
			case rawTagSubIFDs:
				if count == 1 {
					subIFDOffsets = append(subIFDOffsets, e.read4())
				} else {
					// Value field is an offset to the array of offsets.
					arrayOffset := e.read4()
					e.preservePos(func() error {
						e.seek(int64(arrayOffset))
						for range count {
							subIFDOffsets = append(subIFDOffsets, e.read4())
						}
						return nil
					})
				}
			case rawTagDefaultCropSize:
				w, h, ok := e.readDefaultCropSize(dataType, count)
				if ok {
					hasDefaultCrop = true
					defaultCropW, defaultCropH = w, h
				}
			default:
				e.skip(4)
			}
		}

		// Follow ExifIFD for ExifImageWidth/ExifImageHeight.
		var exifW, exifH int
		if exifIFDOffset > 0 {
			e.preservePos(func() error {
				e.seek(int64(exifIFDOffset))
				exifW, exifH = e.readIFDDimensions(rawTagExifImageWidth, rawTagExifImageHeight)
				return nil
			})
		}

		// Follow SubIFDs for largest dimensions and DefaultCropSize.
		var subW, subH int
		for _, off := range subIFDOffsets {
			e.preservePos(func() error {
				e.seek(int64(off))
				w, h, cropW, cropH := e.readSubIFDInfo()
				if cropW > 0 && cropH > 0 {
					hasDefaultCrop = true
					defaultCropW, defaultCropH = cropW, cropH
				}
				if w*h > subW*subH {
					subW, subH = w, h
				}
				return nil
			})
		}

		// Priority: DefaultCropSize > largest of (ExifIFD, SubIFD, IFD0).
		bestW, bestH := width, height
		if exifW*exifH > bestW*bestH {
			bestW, bestH = exifW, exifH
		}
		if subW*subH > bestW*bestH {
			bestW, bestH = subW, subH
		}
		if hasDefaultCrop && defaultCropW > 0 && defaultCropH > 0 {
			bestW, bestH = defaultCropW, defaultCropH
		}

		e.result.ImageConfig = ImageConfig{
			Width:  bestW,
			Height: bestH,
		}

		// If only CONFIG was requested, we're done.
		if e.opts.Sources == CONFIG {
			return nil
		}

		e.seek(ifdPos)
	}

	dec := newMetaDecoderEXIFFromStreamReader(e.streamReader, 0, e.opts)

	if err := dec.decodeTags("IFD0"); err != nil {
		return err
	}

	// Follow IFD1 (thumbnail).
	ifd1Offset := dec.read4()
	if ifd1Offset > 0 {
		dec.seek(int64(ifd1Offset))
		return dec.decodeTags("IFD1")
	}

	return nil
}

// readDefaultCropSize reads a DefaultCropSize tag value from the current IFD entry's
// value field. Handles SHORT, LONG, and RATIONAL data types.
// Returns (width, height, ok).
func (e *imageDecoderRAW) readDefaultCropSize(dataType uint16, count uint32) (int, int, bool) {
	var w, h int
	switch {
	case dataType == 4 && count == 2: // LONG×2
		// Two LONGs don't fit in 4-byte value field; value is an offset.
		cropOffset := e.read4()
		e.preservePos(func() error {
			e.seek(int64(cropOffset))
			w = int(e.read4())
			h = int(e.read4())
			return nil
		})
	case dataType == 3 && count == 2: // SHORT×2
		w = int(e.read2())
		h = int(e.read2())
	case dataType == 5 && count == 2: // RATIONAL×2
		// Two RATIONALs (each 8 bytes) don't fit inline.
		cropOffset := e.read4()
		e.preservePos(func() error {
			e.seek(int64(cropOffset))
			num1 := e.read4()
			den1 := e.read4()
			num2 := e.read4()
			den2 := e.read4()
			if den1 > 0 {
				w = int(num1 / den1)
			}
			if den2 > 0 {
				h = int(num2 / den2)
			}
			return nil
		})
	default:
		e.skip(4)
		return 0, 0, false
	}
	return w, h, true
}

// readIFDDimensions reads an IFD at the current position and scans for
// width/height tags specified by wTag and hTag. Returns (width, height).
func (e *imageDecoderRAW) readIFDDimensions(wTag, hTag uint16) (int, int) {
	numTags := e.read2()
	var w, h int
	for range int(numTags) {
		tagID := e.read2()
		dataType := e.read2()
		_ = e.read4() // count
		if tagID == wTag || tagID == hTag {
			var value int
			if dataType == 3 { // SHORT
				value = int(e.read2())
				e.skip(2)
			} else { // LONG
				value = int(e.read4())
			}
			if tagID == wTag {
				w = value
			} else {
				h = value
			}
			if w > 0 && h > 0 {
				return w, h
			}
		} else {
			e.skip(4)
		}
	}
	return w, h
}

// readSubIFDInfo reads a SubIFD at the current position and scans for
// ImageWidth, ImageHeight, and DefaultCropSize.
// Returns (width, height, defaultCropW, defaultCropH).
func (e *imageDecoderRAW) readSubIFDInfo() (int, int, int, int) {
	numTags := e.read2()
	var w, h, cropW, cropH int
	for range int(numTags) {
		tagID := e.read2()
		dataType := e.read2()
		count := e.read4()
		switch tagID {
		case rawTagImageWidth, rawTagImageHeight:
			var value int
			if dataType == 3 { // SHORT
				value = int(e.read2())
				e.skip(2)
			} else { // LONG
				value = int(e.read4())
			}
			if tagID == rawTagImageWidth {
				w = value
			} else {
				h = value
			}
		case rawTagDefaultCropSize:
			cw, ch, ok := e.readDefaultCropSize(dataType, count)
			if ok {
				cropW, cropH = cw, ch
			}
		default:
			e.skip(4)
		}
	}
	return w, h, cropW, cropH
}
