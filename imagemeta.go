package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
)

// UnknownPrefix is used as prefix for unknown tags.
const UnknownPrefix = "UnknownTag_"

const (
	// TagSourceEXIF is the EXIF tag source.
	TagSourceEXIF TagSource = 1 << iota
	// TagSourceIPTC is the IPTC tag source.
	TagSourceIPTC
	// TagSourceXMP is the XMP tag source.
	TagSourceXMP
)

var (
	// ErrStopWalking is a sentinel error to signal that the walk should stop.
	ErrStopWalking = fmt.Errorf("stop walking")

	// Internal error to signal that we should stop any further processing.
	errStop = fmt.Errorf("stop")
)

const (
	// ImageFormatAuto signals that the image format should be detected automatically (not implemented yet).
	ImageFormatAuto ImageFormat = iota
	// ImageFormatJPEG is the JPEG image format.
	ImageFormatJPEG
	// ImageFormatTIFF is the TIFF image format.
	ImageFormatTIFF
	// ImageFormatPNG is the PNG image format.
	ImageFormatPNG
	// ImageFormatWebP is the WebP image format.
	ImageFormatWebP
)

// Decode reads EXIF and IPTC metadata from r and returns a Meta struct.
func Decode(opts Options) (err error) {

	if opts.R == nil {
		return fmt.Errorf("need a reader")
	}
	if opts.ImageFormat == ImageFormatAuto {
		return fmt.Errorf("need an image format; format detection not implemented yet")
	}
	if opts.HandleTag == nil {
		opts.HandleTag = func(TagInfo) error { return nil }
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
				err = base.streamErr()
			}

		}
	}()

	var dec decoder

	switch opts.ImageFormat {
	case ImageFormatJPEG:
		dec = &imageDecoderJPEG{baseStreamingDecoder: base}
	case ImageFormatTIFF:
		dec = &imageDecoderTIF{baseStreamingDecoder: base}
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
	return err
}

// HandleTagFunc is the function that is called for each tag.
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

	// The default XMP handler is currently very simple:
	// It decodes the RDF.Description.Attrs using Go's xml package and passes each tag to HandleTag.
	// If HandleXMP is set, the decoder will call this function for each XMP packet instead.
	// Note that r must be read completely.
	HandleXMP func(r io.Reader) error

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

// Remove removes the given source.
func (t TagSource) Remove(source TagSource) TagSource {
	t &= ^source
	return t
}

// IsZero returns true if the source is zero.
func (t TagSource) IsZero() bool {
	return t == 0
}
