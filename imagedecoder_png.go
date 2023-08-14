package imagemeta

type imageDecoderPNG struct {
	*baseStreamingDecoder
}

func (e *imageDecoderPNG) decode() error {
	// http://ftp-osl.osuosl.org/pub/libpng/documents/pngext-1.5.0.html#C.eXIf
	// The four-byte chunk type field contains the decimal values:
	// 101 88 73 102 (ASCII "eXIf")
	// The data segment of the eXIf chunk contains an Exif profile in the format specified in "4.7.2 Interoperability Structure of APP1 in Compressed Data"
	// of [CIPA DC-008-2016] except that the JPEG APP1 marker, length, and the "Exif ID code" described in 4.7.2(C), i.e., "Exif", NULL, and padding byte, are not included.

	// The eXIf chunk may appear anywhere between the IHDR and IEND chunks except between IDAT chunks.
	// The eXIf chunk size is constrained only by the maximum of 2^31-1 bytes imposed by the PNG specification. Only one eXIf chunk is allowed in a PNG datastream.

	// Skip header.
	e.skip(8)
	for {
		chunkLength, typ := e.read4(), e.read4()

		if typ == pngEXIFMarker {
			return func() error {
				r, err := e.bufferedReader(int(chunkLength))
				if err != nil {
					return err
				}
				defer r.Close()
				exifr := newMetaDecoderEXIF(r, e.opts.HandleTag)
				return exifr.decode()
			}()

		}
		e.skip(int64(chunkLength))
		e.skip(4) // skip CRC

	}

}
