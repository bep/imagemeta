package imagemeta

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
)

type imageDecoderPNG struct {
	*baseStreamingDecoder
}

// See https://exiftool.org/TagNames/PNG.html
var (
	pngTagIDExif          = []byte("eXIf")
	pngCompressedText     = []byte("zTXt") // See https://exiftool.org/forum/index.php?topic=7988.msg40759#msg40759
	pngRawProfileTypeIPTC = []byte("Raw profile type iptc")
	pngRawProfileTypeEXIF = []byte("Raw profile type exif")
)

func (e *imageDecoderPNG) decode() error {
	// Skip header.
	e.skip(8)

	// Keyword is 1-79 bytes, followed by the 1 byte null character,
	// but we're only interested in a sub set, both 21 in length,
	// so make it simple for now.
	const zTXtKeywordLength = 21

	sources := e.opts.Sources

	skipTag := func(chunkLength uint32) {
		e.skip(int64(chunkLength))
		e.skip(4) // skip CRC
	}

	for {
		if sources.IsZero() {
			return nil
		}
		chunkLength := e.read4()
		tagID := e.readBytesVolatile(4)
		if sources.Has(EXIF) && bytes.Equal(tagID, pngTagIDExif) {
			sources = sources.Remove(EXIF)
			if err := func() error {
				r, err := e.bufferedReader(int64(chunkLength))
				if err != nil {
					return err
				}
				defer r.Close()
				exifr := newMetaDecoderEXIF(r, e.byteOrder, 0, e.opts)
				return exifr.decode()
			}(); err != nil {
				return err
			}
			e.skip(4) // skip CRC
		} else if chunkLength > zTXtKeywordLength && bytes.Equal(tagID, pngCompressedText) {
			keyword := e.readBytesVolatile(zTXtKeywordLength)
			// See https://exiftool.org/forum/index.php?topic=7988.msg40759#msg40759
			if bytes.Equal(keyword, pngRawProfileTypeIPTC) {
				if sources.Has(IPTC) {
					sources = sources.Remove(IPTC)
					data := decompressZTXt(e.readBytesVolatile(int(chunkLength - zTXtKeywordLength)))
					// ImageMagick has different headers, so make this smarter. TODO1
					data = data[23:] // Skip the header bytes.
					data = bytes.ReplaceAll(data, []byte("\n"), []byte(""))
					d := make([]byte, hex.DecodedLen(len(data)))
					_, err := hex.Decode(d, data)
					if err != nil {
						return fmt.Errorf("decoding hex: %w", err)
					}
					r := bytes.NewReader(d)
					// TODO1 encoding (test), these are supposed to be ISO-8859-1.
					iptcDec := newMetaDecoderIPTC(r, e.opts)
					if err := iptcDec.decodeBlocks(); err != nil {
						return err
					}
					if err := iptcDec.decodeRecords(); err != nil {
						return err
					}
				} else {
					e.skip(int64(chunkLength - zTXtKeywordLength))
				}
			} else if bytes.Equal(keyword, pngRawProfileTypeEXIF) {
				e.skip(int64(chunkLength - zTXtKeywordLength))
			} else {
				e.skip(int64(chunkLength - zTXtKeywordLength))
			}
			e.skip(4) // skip CRC
		} else {
			skipTag(chunkLength)
		}
	}
}

// TODO1 get rid of the panics.
func decompressZTXt(data []byte) []byte {
	// The first byte after the null indicates the compression method, for which only deflate is currently defined (method zero).
	compressionMethod := data[1]
	if compressionMethod != 0 {
		panic(fmt.Errorf("unknown PNG compression method %v", compressionMethod))
	}
	b := bytes.NewReader(data[2:])
	z, err := zlib.NewReader(b)
	if err != nil {
		panic(err)
	}
	defer z.Close()
	p, err := io.ReadAll(z)
	if err != nil {
		panic(err)
	}
	return p
}
