// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

type imageDecoderGIF struct {
	*baseStreamingDecoder
}

// decode walks the GIF blocks to count the frames without decompressing any image data.
// See https://www.w3.org/Graphics/GIF/spec-gif89a.txt
func (e *imageDecoderGIF) decode() error {
	if !e.opts.Sources.Has(CONFIG) {
		return nil
	}

	var buf [255]byte

	e.readBytes(buf[:6])
	if s := string(buf[:6]); s != "GIF87a" && s != "GIF89a" {
		return errInvalidFormat
	}

	e.result.ImageConfig = ImageConfig{Width: int(e.read2()), Height: int(e.read2())}
	flags := e.read1()
	e.readBytes(buf[:2]) // Background color index, pixel aspect ratio.

	// Read (not skip) to avoid a seek, which resets the read buffer.
	discard := func(n int) {
		for n > 0 && !e.isEOF {
			m := min(n, len(buf))
			e.readBytes(buf[:m])
			n -= m
		}
	}

	discardColorTable := func(flags uint8) {
		if flags&0x80 != 0 {
			discard(3 << (flags&0x07 + 1))
		}
	}

	discardSubBlocks := func() {
		for {
			n := e.read1()
			if n == 0 || e.isEOF {
				return
			}
			discard(int(n))
		}
	}

	discardColorTable(flags)

	for {
		b := e.read1()
		if e.isEOF {
			return nil
		}
		switch b {
		case 0x2C: // Image descriptor.
			e.result.ImageConfig.FrameCount++
			discard(8) // Left, top, width, height.
			discardColorTable(e.read1())
			discard(1) // LZW minimum code size.
			discardSubBlocks()
		case 0x21: // Extension.
			discard(1) // Label.
			discardSubBlocks()
		default: // Trailer (0x3B) or garbage.
			return nil
		}
	}
}
