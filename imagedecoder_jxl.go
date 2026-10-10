// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"bytes"
	"io"
)

// JPEG XL container box types.
var jxlFCC = struct {
	jxl, ftyp, jxlc, jxlp, exif, xml, brob fourCC
}{
	jxl:  fourCC{'J', 'X', 'L', ' '},
	ftyp: fourCC{'f', 't', 'y', 'p'},
	jxlc: fourCC{'j', 'x', 'l', 'c'},
	jxlp: fourCC{'j', 'x', 'l', 'p'},
	exif: fourCC{'E', 'x', 'i', 'f'},
	xml:  fourCC{'x', 'm', 'l', ' '},
	brob: fourCC{'b', 'r', 'o', 'b'},
}

var jxlContainerSignature = []byte{0, 0, 0, 0x0c, 'J', 'X', 'L', ' ', 0x0d, 0x0a, 0x87, 0x0a}

// Codestream signature (2 bytes) + max SizeHeader (68 bits).
const jxlHeaderLen = 11

type imageDecoderJXL struct {
	*baseStreamingDecoder
	sourceSet Source
}

func (e *imageDecoderJXL) decode() error {
	e.sourceSet = EXIF | XMP | CONFIG
	e.sourceSet = e.sourceSet & e.opts.Sources

	var sig [12]byte
	if err := e.readBytes(sig[:2]); err != nil {
		return errInvalidFormat
	}
	if sig[0] == 0xff && sig[1] == 0x0a {
		// Bare codestream, no metadata boxes.
		if e.sourceSet.Has(CONFIG) {
			e.seek(0)
			return e.decodeConfig(e.readUpTo(jxlHeaderLen))
		}
		return nil
	}
	if err := e.readBytes(sig[2:]); err != nil || !bytes.Equal(sig[:], jxlContainerSignature) {
		return errInvalidFormat
	}

	var header jxlCodestreamHeader

	for !e.sourceSet.IsZero() {
		start := e.pos()
		size := uint64(e.read4())
		var boxType fourCC
		e.readBytes(boxType[:])
		if e.isEOF {
			return nil
		}
		if size == 1 {
			size = e.read8r(e.r)
		}
		headerLen := uint64(e.pos() - start)
		if size != 0 && size < headerLen {
			return errInvalidFormat
		}
		payloadLen := int64(size - headerLen)
		if size == 0 {
			// Box extends to EOF.
			payloadLen = maxBufSize
		}
		end := start + int64(size)

		switch boxType {
		case jxlFCC.jxlc, jxlFCC.jxlp:
			if e.sourceSet.Has(CONFIG) {
				var index uint32
				last := boxType == jxlFCC.jxlc || size == 0
				if boxType == jxlFCC.jxlp {
					const lastPartBit = 1 << 31
					index = e.read4()
					last = last || index&lastPartBit != 0
					index &^= lastPartBit
					payloadLen -= 4
				}
				if payloadLen < 0 {
					return errInvalidFormat
				}
				if header.add(index, e.readUpTo(min(payloadLen, jxlHeaderLen)), last) {
					e.sourceSet = e.sourceSet.Remove(CONFIG)
					if err := e.decodeConfig(header.b); err != nil {
						return err
					}
				}
			}
		case jxlFCC.exif:
			if e.sourceSet.Has(EXIF) {
				e.sourceSet = e.sourceSet.Remove(EXIF)
				if err := e.decodeEXIF(e.r, payloadLen, e.pos()); err != nil {
					return err
				}
			}
		case jxlFCC.xml:
			// There may be multiple XMP packets, so keep XMP in the source set.
			if e.sourceSet.Has(XMP) {
				if err := e.decodeXMP(e.r, payloadLen); err != nil {
					return err
				}
			}
		case jxlFCC.brob:
			if err := e.decodeBrob(payloadLen); err != nil {
				return err
			}
		}

		if size == 0 {
			return nil
		}
		e.seek(end)
	}

	return nil
}

func (e *imageDecoderJXL) decodeBrob(payloadLen int64) error {
	var innerType fourCC
	e.readBytes(innerType[:])
	var source Source
	switch innerType {
	case jxlFCC.exif:
		source = EXIF
	case jxlFCC.xml:
		source = XMP
	default:
		return nil
	}
	if !e.sourceSet.Has(source) {
		return nil
	}

	if e.opts.DecompressBrotli == nil {
		e.opts.Warnf("jxl: skipping Brotli compressed %s; set Options.DecompressBrotli to decode it", source)
		return nil
	}
	if source == EXIF {
		e.sourceSet = e.sourceSet.Remove(EXIF)
	}

	compressed, err := e.bufferedReader(payloadLen - 4)
	if err != nil {
		return err
	}
	defer compressed.Close()

	b, err := io.ReadAll(io.LimitReader(e.opts.DecompressBrotli(compressed), maxBufSize+1))
	if err != nil {
		return newInvalidFormatErrorf("jxl: brotli: %s", err)
	}
	if len(b) > maxBufSize {
		return newInvalidFormatErrorf("jxl: decompressed %s exceeds max %d", source, maxBufSize)
	}

	r := bytes.NewReader(b)
	if source == EXIF {
		return e.decodeEXIF(r, int64(len(b)), 0)
	}
	return e.decodeXMP(r, int64(len(b)))
}

// decodeEXIF decodes an Exif box payload, which is prefixed with a 4-byte TIFF header offset.
// payloadPos is the stream position of the payload, or 0 if not applicable (compressed).
func (e *imageDecoderJXL) decodeEXIF(r io.Reader, length, payloadPos int64) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if rerr, ok := r.(error); ok && rerr != errStop {
				err = rerr
			}
		}
	}()

	tiffOffset := int64(e.read4r(r))
	dataLen := length - 4 - tiffOffset
	if dataLen <= 0 || dataLen > maxBufSize {
		return newInvalidFormatErrorf("jxl: invalid exif header offset %d", tiffOffset)
	}
	if _, err := io.CopyN(io.Discard, r, tiffOffset); err != nil {
		return err
	}
	var thumbnailOffset int64
	if payloadPos > 0 {
		thumbnailOffset = payloadPos + 4 + tiffOffset
	}

	data := getBytesAndReader(int(dataLen))
	defer data.Close()
	if _, err := io.ReadFull(r, data.b); err != nil {
		return err
	}
	data.r.Reset(data.b)
	return newMetaDecoderEXIF(data, e.byteOrder, thumbnailOffset, e.opts).decode()
}

func (e *imageDecoderJXL) decodeXMP(r io.Reader, length int64) error {
	return decodeXMP(io.LimitReader(r, length), e.opts)
}

// readUpTo reads at most n bytes, fewer if the stream ends.
func (e *imageDecoderJXL) readUpTo(n int64) []byte {
	b := make([]byte, n)
	m, _ := io.ReadFull(e.r, b)
	return b[:m]
}

// decodeConfig reads the image dimensions from the start of the codestream.
func (e *imageDecoderJXL) decodeConfig(b []byte) error {
	if len(b) < 2 || b[0] != 0xff || b[1] != 0x0a {
		return errInvalidFormat
	}
	br := &jxlBitReader{b: b[2:]}

	readDim := func() int {
		bits := [4]uint{9, 13, 18, 30}[br.read(2)]
		return int(br.read(bits)) + 1
	}

	var width, height int
	small := br.read(1) == 1
	if small {
		height = (int(br.read(5)) + 1) * 8
	} else {
		height = readDim()
	}
	ratio := br.read(3)
	switch ratio {
	case 0:
		if small {
			width = (int(br.read(5)) + 1) * 8
		} else {
			width = readDim()
		}
	default:
		r := [8][2]int{{}, {1, 1}, {12, 10}, {4, 3}, {3, 2}, {16, 9}, {5, 4}, {2, 1}}[ratio]
		width = height * r[0] / r[1]
	}

	e.result.ImageConfig = ImageConfig{Width: width, Height: height}
	return nil
}

// jxlCodestreamHeader collects the first jxlHeaderLen bytes of a codestream
// split across jxlp boxes, which may be stored out of order.
type jxlCodestreamHeader struct {
	b       []byte
	next    uint32
	pending map[uint32]jxlCodestreamPart
}

type jxlCodestreamPart struct {
	b    []byte
	last bool
}

// add adds the start of the payload of part index (at most jxlHeaderLen bytes).
// It returns true when the header is complete or the codestream has ended.
func (h *jxlCodestreamHeader) add(index uint32, b []byte, last bool) bool {
	const maxPending = 64
	if index < h.next || len(h.pending) >= maxPending {
		return false
	}
	if h.pending == nil {
		h.pending = make(map[uint32]jxlCodestreamPart)
	}
	h.pending[index] = jxlCodestreamPart{b: b, last: last}

	for {
		p, ok := h.pending[h.next]
		if !ok {
			return false
		}
		delete(h.pending, h.next)
		h.next++
		h.b = append(h.b, p.b[:min(len(p.b), jxlHeaderLen-len(h.b))]...)
		if p.last || len(h.b) == jxlHeaderLen {
			return true
		}
	}
}

// jxlBitReader reads bits LSB first.
type jxlBitReader struct {
	b   []byte
	pos uint
}

func (r *jxlBitReader) read(n uint) uint32 {
	var v uint32
	for i := range n {
		byteIdx := r.pos / 8
		if byteIdx >= uint(len(r.b)) {
			panic(errInvalidFormat)
		}
		bit := (r.b[byteIdx] >> (r.pos % 8)) & 1
		v |= uint32(bit) << i
		r.pos++
	}
	return v
}
