package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// UnknownPrefix is used as prefix for unknown tags.
const UnknownPrefix = "UnknownTag_"

const (
	// EXIF is the EXIF tag source.
	EXIF TagSource = 1 << iota
	// IPTC is the IPTC tag source.
	IPTC
	// XMP is the XMP tag source.
	XMP
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
	// JPEG is the JPEG image format.
	JPEG
	// TIFF is the TIFF image format.
	TIFF
	// PNG is the PNG image format.
	PNG
	// WebP is the WebP image format.
	WebP
)

// Decode reads EXIF and IPTC metadata from r and returns a Meta struct.
func Decode(opts Options) (err error) {
	if opts.R == nil {
		return fmt.Errorf("no reader provided")
	}
	if opts.ImageFormat == ImageFormatAuto {
		return fmt.Errorf("no image format provided; format detection not implemented yet")
	}
	if opts.ShouldHandleTag == nil {
		opts.ShouldHandleTag = func(ti TagInfo) bool {
			if ti.Source != EXIF {
				return true
			}
			// Skip all tags in the thumbnails IFD (IFD1).
			return strings.HasPrefix(ti.Namespace, "IFD0")
		}
	}

	if opts.HandleTag == nil {
		opts.HandleTag = func(TagInfo) error { return nil }
	}

	if opts.Sources == 0 {
		opts.Sources = EXIF | IPTC | XMP
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

			if err == io.EOF {
				err = nil
			}

		}
	}()

	var dec decoder

	switch opts.ImageFormat {
	case JPEG:
		dec = &imageDecoderJPEG{baseStreamingDecoder: base}
	case TIFF:
		dec = &imageDecoderTIF{baseStreamingDecoder: base}
	case WebP:
		base.byteOrder = binary.LittleEndian
		dec = &decoderWebP{baseStreamingDecoder: base}
	case PNG:
		dec = &imageDecoderPNG{baseStreamingDecoder: base}
	default:
		return fmt.Errorf("unsupported image format")

	}

	err = dec.decode()

	if err == ErrStopWalking {
		return nil
	}

	if err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return nil
}

// HandleTagFunc is the function that is called for each tag.
type HandleTagFunc func(info TagInfo) error

// ImageFormat is the image format.
//
//go:generate stringer -type=ImageFormat
type ImageFormat int

// Options contains the options for the Decode function.
type Options struct {
	// The Reader (typically a *os.File) to read image metadata from.
	R Reader

	// The image format in R.
	ImageFormat ImageFormat

	// If set, the decoder skip tags in which this function returns false.
	// If not set, a default function is used that skips all EXIF tags except those in IFD0.
	ShouldHandleTag func(tag TagInfo) bool

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
	// The tag namespace.
	// For EXIF, this is the path to the IFD, e.g. "IFD0/GPSInfoIFD"
	// For XMP, this is the namespace, e.g. "http://ns.adobe.com/camera-raw-settings/1.0/"
	// For IPTC, this is the record tag name as defined https://exiftool.org/TagNames/IPTC.html
	Namespace string
	// The tag value.
	Value any
}

// TagSource is a bitmask and you may send multiple sources at once.
//
//go:generate stringer -type=TagSource
type TagSource uint32

// Remove removes the given source.
func (t TagSource) Remove(source TagSource) TagSource {
	t &= ^source
	return t
}

// Has returns true if the given source is set.
func (t TagSource) Has(source TagSource) bool {
	return t&source != 0
}

// IsZero returns true if the source is zero.
func (t TagSource) IsZero() bool {
	return t == 0
}

// Tags is a collection of tags grouped per source.
type Tags struct {
	exif map[string]TagInfo
	iptc map[string]TagInfo
	xmp  map[string]TagInfo
}

// Add adds a tag to the correct source.
func (t *Tags) Add(tag TagInfo) {
	t.getSourceMap(tag.Source)[tag.Tag] = tag
}

// Has reports if a tag is already added.
func (t *Tags) Has(tag TagInfo) bool {
	_, found := t.getSourceMap(tag.Source)[tag.Tag]
	return found
}

// EXIF returns the EXIF tags.
func (t *Tags) EXIF() map[string]TagInfo {
	if t.exif == nil {
		t.exif = make(map[string]TagInfo)
	}
	return t.exif
}

// IPTC returns the IPTC tags.
func (t *Tags) IPTC() map[string]TagInfo {
	if t.iptc == nil {
		t.iptc = make(map[string]TagInfo)
	}
	return t.iptc
}

// XMP returns the XMP tags.
func (t *Tags) XMP() map[string]TagInfo {
	if t.xmp == nil {
		t.xmp = make(map[string]TagInfo)
	}
	return t.xmp
}

// All returns all tags in a map.
func (t Tags) All() map[string]TagInfo {
	all := make(map[string]TagInfo)
	for k, v := range t.EXIF() {
		all[k] = v
	}
	for k, v := range t.IPTC() {
		all[k] = v
	}
	for k, v := range t.XMP() {
		all[k] = v
	}
	return all
}

// GetDateTime tries DateTimeOriginal and then DateTime,
// in the EXIF tags, and returns the parsed time.Time value if found.
func (t Tags) GetDateTime() (time.Time, error) {
	dateStr := t.dateTime()
	if dateStr == "" {
		return time.Time{}, nil
	}

	loc := time.Local
	if v := t.location(); v != nil {
		loc = v
	}

	const layout = "2006:01:02 15:04:05"

	return time.ParseInLocation(layout, dateStr, loc)
}

// GetLatLong returns the latitude and longitude from the EXIF GPS tags.
func (t Tags) GetLatLong() (lat float64, long float64, err error) {
	var ns, ew string

	exif := t.EXIF()

	longTag, found := exif["GPSLongitude"]
	if !found {
		return
	}
	ewTag, found := exif["GPSLongitudeRef"]
	if found {
		ew = ewTag.Value.(string)
	}
	latTag, found := exif["GPSLatitude"]
	if !found {
		return
	}
	nsTag, found := exif["GPSLatitudeRef"]
	if found {
		ns = nsTag.Value.(string)
	}

	lat = latTag.Value.(float64)
	long = longTag.Value.(float64)

	if ns == "S" {
		lat = -lat
	}

	if ew == "W" {
		long = -long
	}

	if math.IsNaN(lat) {
		lat = 0
	}
	if math.IsNaN(long) {
		long = 0
	}

	return
}

func (t *Tags) getSourceMap(source TagSource) map[string]TagInfo {
	switch source {
	case EXIF:
		return t.EXIF()
	case IPTC:
		return t.IPTC()
	case XMP:
		return t.XMP()
	default:
		return nil
	}
}

func (t Tags) dateTime() string {
	exif := t.EXIF()
	if ti, ok := exif["DateTimeOriginal"]; ok {
		return ti.Value.(string)
	}
	if ti, ok := exif["DateTime"]; ok {
		return ti.Value.(string)
	}
	return ""
}

// Borrowed from github.com/rwcarlsen/goexif
// TODO(bep: look for timezone offset, GPS time, etc.
func (t Tags) location() *time.Location {
	exif := t.EXIF()
	timeInfo, found := exif["Canon.TimeInfo"]
	if !found {
		return nil
	}
	// TODO1 test etc.
	vals := timeInfo.Value.([]uint32)
	if len(vals) < 2 {
		return nil
	}

	return time.FixedZone("", int(vals[1]*60))
}
