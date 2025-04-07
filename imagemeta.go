// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
	"maps"
	"math"
	"strings"
	"time"
)

// UnknownPrefix is used as prefix for unknown tags.
const UnknownPrefix = "UnknownTag_"

const (
	// EXIF is the EXIF tag source.
	EXIF Source = 1 << iota
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
	var base *baseStreamingDecoder

	errFinal := func(err2 error) error {
		if err2 == nil {
			return nil
		}

		if err2 == ErrStopWalking {
			return nil
		}

		if err2 == errStop {
			return nil
		}

		if err2 == nil {
			if base != nil {
				err2 = base.streamErr()
			}
		}

		if err2 == nil {
			return nil
		}

		if err2 == io.EOF {
			return nil
		}

		if isInvalidFormatErrorCandidate(err2) {
			err2 = newInvalidFormatError(err2)
		}

		return err2
	}

	defer func() {
		err = errFinal(err)
	}()

	errFromRecover := func(r any) (err2 error) {
		if r == nil {
			return nil
		}
		if errp, ok := r.(error); ok {
			if isInvalidFormatErrorCandidate(errp) {
				err2 = newInvalidFormatError(errp)
			} else {
				err2 = errp
			}
		} else {
			err2 = fmt.Errorf("unknown panic: %v", r)
		}

		return
	}

	defer func() {
		err2 := errFromRecover(recover())
		if err == nil {
			err = err2
		}
	}()

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

	const (
		defaultLimitNumTags = 5000
		defaultLimitTagSize = 10000
	)

	if opts.LimitNumTags == 0 {
		opts.LimitNumTags = defaultLimitNumTags
	}
	if opts.LimitTagSize == 0 {
		opts.LimitTagSize = defaultLimitTagSize
	}

	var tagCount uint32
	shouldHandleTag := opts.ShouldHandleTag
	opts.ShouldHandleTag = func(ti TagInfo) bool {
		tagCount++
		if tagCount > opts.LimitNumTags {
			panic(ErrStopWalking)
		}
		return shouldHandleTag(ti)
	}

	if opts.HandleTag == nil {
		opts.HandleTag = func(TagInfo) error { return nil }
	}

	if opts.Sources == 0 {
		opts.Sources = EXIF | IPTC | XMP
	}

	if opts.Warnf == nil {
		opts.Warnf = func(string, ...any) {}
	}

	var sourceSet Source

	// Remove sources not supported by the format.
	switch opts.ImageFormat {
	case JPEG:
		sourceSet = EXIF | XMP | IPTC
	case TIFF:
		sourceSet = EXIF | XMP | IPTC
	case WebP:
		sourceSet = EXIF | XMP
	case PNG:
		sourceSet = EXIF | XMP | IPTC
	default:
		return fmt.Errorf("unsupported image format")

	}
	// Remove sources that are not requested.
	sourceSet = sourceSet & opts.Sources
	opts.Sources = sourceSet

	if opts.Sources.IsZero() {
		return nil
	}

	br := &streamReader{
		r:         opts.R,
		byteOrder: binary.BigEndian,
	}

	base = &baseStreamingDecoder{
		streamReader: br,
		opts:         opts,
	}

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
	}

	decode := func() chan error {
		errc := make(chan error, 1)
		go func() {
			defer func() {
				err2 := errFromRecover(recover())
				if err2 != nil {
					errc <- err2
				}
			}()
			errc <- dec.decode()
		}()
		return errc
	}

	if opts.Timeout > 0 {
		select {
		case <-time.After(opts.Timeout):
			err = fmt.Errorf("timed out after %s", opts.Timeout)
		case err = <-decode():
		}
	} else {
		err = dec.decode()
	}

	return
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
	R io.ReadSeeker

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
	Sources Source

	// Warnf will be called for each warning.
	Warnf func(string, ...any)

	// Timeout is the maximum time the decoder will spend on reading metadata.
	// Mostly useful for testing.
	// If set to 0, the decoder will not time out.
	Timeout time.Duration

	// LimitNumTags is the maximum number of tags to read.
	// Default value is 5000.
	LimitNumTags uint32

	// LimitTagSize is the maximum size in bytes of a tag value to read.
	// Tag values larger than this will be skipped without notice.
	// Note that this limit is not relevant for the XMP source.
	// Default value is 10000.
	LimitTagSize uint32
}

// TagInfo contains information about a tag.
type TagInfo struct {
	// The tag source.
	Source Source
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

// Source is a bitmask and you may send multiple sources at once.
//
//go:generate stringer -type=Source
type Source uint32

// Remove removes the given source.
func (t Source) Remove(source Source) Source {
	t &= ^source
	return t
}

// Has returns true if the given source is set.
func (t Source) Has(source Source) bool {
	return t&source != 0
}

// IsZero returns true if the source is zero.
func (t Source) IsZero() bool {
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
	maps.Copy(all, t.EXIF())
	maps.Copy(all, t.IPTC())
	maps.Copy(all, t.XMP())
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

func (t *Tags) getSourceMap(source Source) map[string]TagInfo {
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
	vals := timeInfo.Value.([]uint32)
	if len(vals) < 2 {
		return nil
	}

	return time.FixedZone("", int(vals[1]*60))
}

type baseStreamingDecoder struct {
	*streamReader
	opts Options
	err  error
}

func (d *baseStreamingDecoder) streamErr() error {
	if d.err != nil {
		return d.err
	}
	return d.readErr
}

type decoderWebP struct {
	*baseStreamingDecoder
}
