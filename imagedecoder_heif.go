// Copyright 2026 Toni Melisma
// SPDX-License-Identifier: MIT

package imagemeta

import "math"

// ISOBMFF box and item types used in HEIF/AVIF containers.
var (
	fccFtyp = fourCC{'f', 't', 'y', 'p'}
	fccMeta = fourCC{'m', 'e', 't', 'a'}
	fccIinf = fourCC{'i', 'i', 'n', 'f'}
	fccInfe = fourCC{'i', 'n', 'f', 'e'}
	fccIloc = fourCC{'i', 'l', 'o', 'c'}
	fccIprp = fourCC{'i', 'p', 'r', 'p'}
	fccIpco = fourCC{'i', 'p', 'c', 'o'}
	fccIpma = fourCC{'i', 'p', 'm', 'a'}
	fccIspe = fourCC{'i', 's', 'p', 'e'}
	fccIrot = fourCC{'i', 'r', 'o', 't'}
	fccPitm = fourCC{'p', 'i', 't', 'm'}
	fccExif = fourCC{'E', 'x', 'i', 'f'}
	fccMime = fourCC{'m', 'i', 'm', 'e'}
)

type imageDecoderHEIF struct {
	*baseStreamingDecoder
}

func (e *imageDecoderHEIF) decode() error {
	// readVarUint reads n bytes from the stream as a big-endian uint64.
	// n must be 0, 2, 4, or 8. Returns 0 for n == 0.
	readVarUint := func(n int) uint64 {
		switch n {
		case 0:
			return 0
		case 2:
			return uint64(e.read2())
		case 4:
			return uint64(e.read4())
		case 8:
			return e.read8r(e.r)
		default:
			panic(newInvalidFormatErrorf("heif: unsupported iloc field size: %d", n))
		}
	}

	// readBox reads an ISOBMFF box header from the current stream position.
	// Returns (startPos, totalBoxSize, boxType).
	// startPos: absolute stream position before the header.
	// totalBoxSize: total box size including header bytes (0 = extends to EOF).
	// After this call, the stream is positioned at the start of the box payload.
	readBox := func() (startPos int64, totalSize uint64, boxType fourCC) {
		startPos = e.pos()
		size := e.read4()
		e.readBytes(boxType[:])
		totalSize = uint64(size)
		if size == 1 {
			// Extended size: next 8 bytes hold the actual size.
			totalSize = e.read8r(e.r)
		}
		return
	}

	// Step 1: Read and validate the ftyp box.
	ftypStart, ftypSize, ftypType := readBox()
	if e.isEOF {
		return errInvalidFormat
	}
	if ftypType != fccFtyp {
		return errInvalidFormat
	}
	if ftypSize > 0 {
		e.seek(ftypStart + int64(ftypSize))
	}

	// Step 2: Scan top-level boxes for the meta box.
	var (
		metaStart int64
		metaSize  uint64
	)
	for {
		s, size, boxType := readBox()
		if e.isEOF {
			return nil // No meta box found; nothing to decode.
		}
		if boxType == fccMeta {
			metaStart = s
			metaSize = size
			break
		}
		if size == 0 {
			return nil // Box extends to EOF; no meta found.
		}
		e.seek(s + int64(size))
	}

	// Step 3: Parse the meta FullBox (skip 4 bytes version+flags).
	e.skip(4)

	var metaEnd int64
	if metaSize == 0 {
		metaEnd = math.MaxInt64 // extends to EOF
	} else {
		metaEnd = metaStart + int64(metaSize)
	}

	var (
		exifItemID    uint32
		xmpItemID     uint32
		primaryItemID uint32
	)

	// iloc entries keyed by item ID, resolved after the full meta scan
	// so that box ordering (iloc before/after iinf) doesn't matter.
	type ilocEntry struct {
		offset, length uint64
	}
	ilocEntries := make(map[uint32]ilocEntry)

	// For CONFIG: ipco properties and ipma associations are collected during
	// the meta box scan and resolved afterwards, so box ordering doesn't matter.
	type ipcoProp struct {
		isIspe        bool
		isIrot        bool
		width, height uint32
		angle         uint8
	}
	var ipcoProps []ipcoProp
	var primaryPropIndices []int // 1-based property indices from ipma

	// Step 4: Iterate inner boxes of meta to find pitm, iinf, iloc, and iprp.
	for e.pos()+8 <= metaEnd {
		innerStart, innerSize, innerType := readBox()
		if e.isEOF {
			break
		}
		if innerSize == 0 {
			break // extends to EOF
		}
		innerEnd := innerStart + int64(innerSize)

		switch innerType {
		case fccPitm:
			if e.opts.Sources.Has(CONFIG) {
				vf := e.read4()
				if vf>>24 == 0 {
					primaryItemID = uint32(e.read2())
				} else {
					primaryItemID = e.read4()
				}
			}

		case fccIinf:
			// iinf is a FullBox: read version+flags then item count.
			vf := e.read4()
			iinfVersion := vf >> 24
			var count uint32
			if iinfVersion == 0 {
				count = uint32(e.read2())
			} else {
				count = e.read4()
			}

			// Iterate infe sub-boxes.
			for range count {
				infeStart, infeSize, infeType := readBox()
				if e.isEOF || infeSize == 0 {
					break
				}
				infeEnd := infeStart + int64(infeSize)

				if infeType == fccInfe {
					// infe is a FullBox: read version+flags.
					vf2 := e.read4()
					infeVersion := vf2 >> 24
					if infeVersion >= 2 {
						var itemID uint32
						if infeVersion == 2 {
							itemID = uint32(e.read2())
						} else {
							// Version 3: 32-bit item ID.
							itemID = e.read4()
						}
						e.skip(2) // protectionIndex
						var itemType fourCC
						e.readBytes(itemType[:])
						switch itemType {
						case fccExif:
							exifItemID = itemID
						case fccMime:
							xmpItemID = itemID
						}
					} else {
						e.opts.Warnf("heif: infe version %d not supported, skipping", infeVersion)
					}
				}
				e.seek(infeEnd)
			}

		case fccIloc:
			// iloc is a FullBox: read version+flags.
			vf := e.read4()
			ilocVersion := uint8(vf >> 24)

			b1 := e.read1()
			offsetSize := int(b1 >> 4)
			lengthSize := int(b1 & 0x0f)

			b2 := e.read1()
			baseOffsetSize := int(b2 >> 4)
			indexSize := int(b2 & 0x0f)

			var count uint32
			if ilocVersion < 2 {
				count = uint32(e.read2())
			} else {
				count = e.read4()
			}

			for range count {
				var itemID uint32
				if ilocVersion < 2 {
					itemID = uint32(e.read2())
				} else {
					itemID = e.read4()
				}

				var constructionMethod uint16
				if ilocVersion >= 1 {
					constructionMethod = e.read2()
				}
				e.skip(2) // dataReferenceIndex

				baseOffset := readVarUint(baseOffsetSize)

				extentCount := e.read2()

				// Only file-offset construction (method 0) is supported.
				if constructionMethod != 0 {
					for range extentCount {
						if ilocVersion >= 1 && indexSize > 0 {
							readVarUint(indexSize)
						}
						readVarUint(offsetSize)
						readVarUint(lengthSize)
					}
					continue
				}

				var firstOffset, firstLength uint64
				for j := range extentCount {
					if ilocVersion >= 1 && indexSize > 0 {
						readVarUint(indexSize) // extent index, discard
					}
					off := readVarUint(offsetSize)
					length := readVarUint(lengthSize)
					if j == 0 {
						firstOffset = baseOffset + off
						firstLength = length
					}
				}

				ilocEntries[itemID] = ilocEntry{offset: firstOffset, length: firstLength}
			}

		case fccIprp:
			if e.opts.Sources.Has(CONFIG) {
				iprpEnd := innerEnd
				for e.pos()+8 <= iprpEnd {
					childStart, childSize, childType := readBox()
					if e.isEOF || childSize == 0 {
						break
					}
					childEnd := childStart + int64(childSize)

					switch childType {
					case fccIpco:
						for e.pos()+8 <= childEnd {
							propStart, propSize, propType := readBox()
							if e.isEOF || propSize == 0 {
								break
							}
							propEnd := propStart + int64(propSize)

							var prop ipcoProp
							switch propType {
							case fccIspe:
								e.skip(4) // version+flags
								prop = ipcoProp{isIspe: true, width: e.read4(), height: e.read4()}
							case fccIrot:
								prop = ipcoProp{isIrot: true, angle: e.read1()}
							}
							ipcoProps = append(ipcoProps, prop)
							e.seek(propEnd)
						}

					case fccIpma:
						// ipma maps item IDs to property indices in ipco.
						vf := e.read4()
						ipmaVersion := uint8(vf >> 24)
						ipmaFlags := vf & 0xFFFFFF
						entryCount := e.read4()
						for range entryCount {
							var itemID uint32
							if ipmaVersion < 1 {
								itemID = uint32(e.read2())
							} else {
								itemID = e.read4()
							}
							assocCount := e.read1()
							for range assocCount {
								var propIdx int
								if ipmaFlags&1 != 0 {
									val := e.read2()
									propIdx = int(val & 0x7FFF)
								} else {
									val := e.read1()
									propIdx = int(val & 0x7F)
								}
								if itemID == primaryItemID && primaryItemID != 0 {
									primaryPropIndices = append(primaryPropIndices, propIdx)
								}
							}
						}
					}
					e.seek(childEnd)
				}
			}
		}

		// Always advance to the end of this inner box.
		e.seek(innerEnd)
	}

	// Step 5: Resolve iloc offsets now that both iinf and iloc have been parsed.
	var exifOffset, exifLength, xmpOffset, xmpLength uint64
	if loc, ok := ilocEntries[exifItemID]; ok && exifItemID != 0 {
		exifOffset, exifLength = loc.offset, loc.length
	}
	if loc, ok := ilocEntries[xmpItemID]; ok && xmpItemID != 0 {
		xmpOffset, xmpLength = loc.offset, loc.length
	}

	// Step 6: Resolve CONFIG dimensions from collected ipco/ipma/pitm data.
	if e.opts.Sources.Has(CONFIG) && len(ipcoProps) > 0 {
		var cfgWidth, cfgHeight uint32
		var cfgRotate bool

		if primaryItemID != 0 && len(primaryPropIndices) > 0 {
			// Primary path: use pitm + ipma to find the primary item's properties.
			for _, idx := range primaryPropIndices {
				if idx < 1 || idx > len(ipcoProps) {
					continue
				}
				p := ipcoProps[idx-1]
				if p.isIspe && p.width > 0 && p.height > 0 {
					cfgWidth, cfgHeight = p.width, p.height
				}
				if p.isIrot && (p.angle == 1 || p.angle == 3) {
					cfgRotate = true
				}
			}
		}

		if cfgWidth == 0 || cfgHeight == 0 {
			// Fallback: use the largest ispe (primary image is always larger
			// than tiles or thumbnails in standard HEIF/AVIF output).
			for _, p := range ipcoProps {
				if p.isIspe && p.width > 0 && p.height > 0 {
					if uint64(p.width)*uint64(p.height) > uint64(cfgWidth)*uint64(cfgHeight) {
						cfgWidth, cfgHeight = p.width, p.height
					}
				}
			}
			for _, p := range ipcoProps {
				if p.isIrot && (p.angle == 1 || p.angle == 3) {
					cfgRotate = true
					break
				}
			}
		}

		if cfgWidth > 0 && cfgHeight > 0 {
			if cfgRotate {
				cfgWidth, cfgHeight = cfgHeight, cfgWidth
			}
			e.result.ImageConfig = ImageConfig{Width: int(cfgWidth), Height: int(cfgHeight)}
		}
	}

	// Step 7: Extract EXIF metadata using the absolute offset from iloc.
	if e.opts.Sources.Has(EXIF) && exifItemID != 0 && exifOffset != 0 && exifLength > 4 {
		if err := e.handleEXIF(exifOffset, exifLength); err != nil {
			return err
		}
	}

	// Step 8: Extract XMP metadata using the absolute offset from iloc.
	if e.opts.Sources.Has(XMP) && xmpItemID != 0 && xmpOffset != 0 && xmpLength > 0 {
		if err := func() error {
			e.seek(int64(xmpOffset))
			r, err := e.bufferedReader(int64(xmpLength))
			if err != nil {
				return err
			}
			defer r.Close()
			return decodeXMP(r, e.opts)
		}(); err != nil {
			return err
		}
	}

	return nil
}

func (e *imageDecoderHEIF) handleEXIF(offset, length uint64) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// Recover from panic in EXIF decoder (e.g., errStop).
			// This allows the HEIF decoder to continue processing XMP.
			if rerr, ok := r.(error); ok && rerr != errStop {
				err = rerr
			}
		}
	}()

	e.seek(int64(offset))
	// HEIF EXIF blobs are prefixed with a 4-byte big-endian header offset.
	// Skip that many bytes to reach the TIFF header.
	exifHdrOffset := e.read4()
	if int64(exifHdrOffset) > int64(length)-4 {
		return newInvalidFormatErrorf("heif: invalid exif header offset %d", exifHdrOffset)
	}
	e.skip(int64(exifHdrOffset))
	thumbnailPos := e.pos()
	dataLen := int64(length) - 4 - int64(exifHdrOffset)
	if dataLen <= 0 {
		return nil
	}
	r, err := e.bufferedReader(dataLen)
	if err != nil {
		return err
	}
	defer r.Close()
	dec := newMetaDecoderEXIF(r, e.byteOrder, thumbnailPos, e.opts)
	return dec.decode()
}
