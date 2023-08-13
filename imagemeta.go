package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"time"
)

const (
	TagSourceEXIF TagSource = iota
	TagSourceIPTC
	TagSourceXMP
)

// Sentinel error to signal that the walk should stop.
var ErrStopWalking = fmt.Errorf("stop walking")

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
	if opts.SourceSet == nil {
		opts.SourceSet = map[TagSource]bool{TagSourceEXIF: true, TagSourceIPTC: true, TagSourceXMP: true}
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

type HandleTagFunc func(info TagInfo) error

//go:generate stringer -type=ImageFormat
type ImageFormat int

// Meta contains the EXIF and IPTC metadata.
// TODO1
type Meta struct {
	EXIF Tags
	IPTC Tags
}

// DateTime returns the DateTime tag as a time.Time parsed with the given location.
// Todo timezone if loc is nil.
func (m Meta) DateTime(loc *time.Location) time.Time {
	tags := m.EXIF
	if v, ok := tags["DateTime"]; ok {
		t, err := time.ParseInLocation("2006:01:02 15:04:05", v.(string), loc)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

// DateTimeOriginal returns the DateTimeOriginal tag as a time.Time parsed with the given location.
func (m Meta) DateTimeOriginal(loc *time.Location) time.Time {
	tags := m.EXIF
	if v, ok := tags["DateTimeOriginal"]; ok {
		t, err := time.ParseInLocation("2006:01:02 15:04:05", v.(string), loc)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

func (m Meta) LatLong() (float64, float64) {
	tags := m.EXIF
	longv := tags.get("GPSLongitude")
	ewv := tags.get("GPSLongitudeRef")
	latv := tags.get("GPSLatitude")
	nsv := tags.get("GPSLatitudeRef")

	fmt.Printf("longv: %v, ewv: %v, latv: %v, nsv: %v\n", longv, ewv, latv, nsv)

	if longv == nil || latv == nil {
		return 0, 0
	}

	long := tags.toDegrees(longv)
	lat := tags.toDegrees(latv)

	if ewv == "W" {
		long *= -1.0
	}
	if nsv == "S" {
		lat *= -1.0
	}

	return lat, long
}

// Orientation returns the Orientation tag.
func (m Meta) Orientation() Orientation {
	tags := m.EXIF
	if v, ok := tags["Orientation"]; ok {
		return Orientation(v.(uint16))
	}
	return OrientationUnspecified
}

type Options struct {
	// The Reader (typically a *os.File) to read image metadata from.
	R Reader

	ImageFormat ImageFormat

	HandleTag HandleTagFunc

	// If set, the decoder will only read the given tag sources
	SourceSet map[TagSource]bool
}

type Orientation int

type Reader interface {
	io.ReadSeeker
	io.ReaderAt
}

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

//go:generate stringer -type=TagSource
type TagSource int

// Tags is a map of EXIF and IPTC tags.
type Tags map[string]any

func (tags Tags) toDegrees(v any) float64 {
	if v == nil {
		return 0
	}
	f := [3]float64{}
	for i, vv := range v.([]interface{}) {
		if i >= 3 {
			break
		}
		r := vv.(*big.Rat)
		f[i], _ = r.Float64()
	}
	// TODO1 other types?
	return f[0] + f[1]/60 + f[2]/3600.0
}

func (tags Tags) get(key string) any {
	if v, ok := tags[key]; ok {
		return v
	}
	return nil
}

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}

type readerCloser interface {
	Reader
	io.Closer
}
