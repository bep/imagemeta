// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

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
		} else if bytes.Equal(tagID, pngCompressedText) {
			// Profile Name is 1-79 bytes, followed by the null character.
			// Note that profileNameLength includes the null character.
			profileName, profileNameLength := e.readNullTerminatedBytes(79 + 1)

			// See https://exiftool.org/forum/index.php?topic=7988.msg40759#msg40759
			if bytes.Equal(profileName, pngRawProfileTypeIPTC) {
				if sources.Has(IPTC) {
					sources = sources.Remove(IPTC)

					dataLen := int(chunkLength) - int(profileNameLength)
					if dataLen < 0 {
						return newInvalidFormatErrorf("invalid data length %d", dataLen)
					}

					// TODO(bep) According to the spec, this should always return Latin-1 encoded text.
					// The image editors out there does not seem to care much about this.
					// See https://github.com/bep/imagemeta/issues/19
					data, err := decompressZTXt(e.readBytesVolatile(dataLen))
					if err != nil {
						return newInvalidFormatError(fmt.Errorf("decompressing zTXt: %w", err))
					}
					data = data[profileNameLength:] // Skip the header bytes.
					data = bytes.ReplaceAll(data, []byte("\n"), []byte(""))
					d := make([]byte, hex.DecodedLen(len(data)))
					_, err = hex.Decode(d, data)
					if err != nil {
						return fmt.Errorf("decoding hex: %w", err)
					}
					r := bytes.NewReader(d)

					iptcDec := newMetaDecoderIPTC(r, e.opts)
					if err := iptcDec.decodeBlocks(); err != nil {
						return err
					}

				} else {
					e.skip(int64(chunkLength) - profileNameLength)
				}
			} else if bytes.Equal(profileName, pngRawProfileTypeEXIF) {
				e.skip(int64(chunkLength) - profileNameLength)
			} else {
				e.skip(int64(chunkLength) - profileNameLength)
			}
			e.skip(4) // skip CRC
		} else {
			skipTag(chunkLength)
		}
	}
}

func decompressZTXt(data []byte) ([]byte, error) {
	// The first byte indicates the compression method, for which only deflate is currently defined (method zero).
	compressionMethod := data[0]
	if compressionMethod != 0 {
		return nil, fmt.Errorf("unknown PNG compression method %v", compressionMethod)
	}
	b := bytes.NewReader(data[1:])
	z, err := zlib.NewReader(b)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	p, err := io.ReadAll(z)
	return p, err
}
