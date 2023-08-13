package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	TagSourceEXIF TagSource = 1 << iota
	TagSourceIPTC
	TagSourceXMP
)

var (
	// Sentinel error to signal that the walk should stop.
	ErrStopWalking = fmt.Errorf("stop walking")

	// Internal error to signal that we should stop any further processing.
	errStop = fmt.Errorf("stop")
)

const (
	ImageFormatAuto ImageFormat = iota
	ImageFormatJPEG
	ImageFormatPNG
	ImageFormatWebP
)

// Decode reads EXIF and IPTC metadata from r and returns a Meta struct.
func Decode(opts Options) (err error) {
	if opts.R == nil {
		return fmt.Errorf("need a reader")
	}
	if opts.HandleTag == nil {
		return fmt.Errorf("need a HandleTag function")
	}
	if opts.ImageFormat == ImageFormatAuto {
		return fmt.Errorf("need an image format; format detection not implemented yet")
	}
	if opts.Sources == 0 {
		opts.Sources = TagSourceEXIF | TagSourceIPTC | TagSourceXMP
	}

	br := &streamReader{
		r:         opts.R,
		byteOrder: binary.BigEndian,
	}

	base := &baseStreamingDecoder{
		streamReader: br,
		opts:         opts,
	}

	defer func() {
		if r := recover(); r != nil {
			if r != errStop {
				panic(r)
			}
			if err == nil {
				err = base.err
			}
		}
	}()

	var dec decoder

	switch opts.ImageFormat {
	case ImageFormatJPEG:
		dec = &imageDecoderJPEG{baseStreamingDecoder: base}
	case ImageFormatWebP:
		base.byteOrder = binary.LittleEndian
		dec = &decoderWebP{baseStreamingDecoder: base}
	case ImageFormatPNG:
		dec = &imageDecoderPNG{baseStreamingDecoder: base}
	default:
		return fmt.Errorf("unsupported image format")

	}

	err = dec.decode()
	if err == ErrStopWalking {
		return nil
	}
	if err != nil {
		return err
	}
	return base.err
}

type HandleTagFunc func(info TagInfo) error

//go:generate stringer -type=ImageFormat
type ImageFormat int

type Options struct {
	// The Reader (typically a *os.File) to read image metadata from.
	R Reader

	// The image format in R.
	ImageFormat ImageFormat

	// The function to call for each tag.
	HandleTag HandleTagFunc

	// If set, the decoder will only read the given tag sources.
	// Note that this is a bitmask and you may send multiple sources at once.
	Sources TagSource
}

// Reader is the interface that wraps the basic Read and Seek methods.
type Reader interface {
	io.ReadSeeker
	io.ReaderAt
}

// TagInfo contains information about a tag.
type TagInfo struct {
	// The tag source.
	Source TagSource
	// The tag name.
	Tag string
	// The tag namespace, if any (currently only set for XMP tags.)
	Namespace string
	// The tag value.
	Value any
}

// TagSource is a bitmask and you may send multiple sources at once.
//
//go:generate stringer -type=TagSource
type TagSource uint32

// Has returns true if the given source is set.
func (t TagSource) Has(source TagSource) bool {
	return t&source != 0
}
