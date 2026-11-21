// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

// ErrMetadataExists is returned by Write when the image already contains EXIF or XMP metadata.
var ErrMetadataExists = errors.New("image already contains metadata")

// Meta is the metadata to write to an image.
// Zero values are not written.
type Meta struct {
	Copyright   string
	Creator     string
	Title       string
	Description string
	Date        time.Time

	// LatLong is the GPS position in decimal degrees.
	LatLong *LatLong
}

// LatLong is a GPS position in decimal degrees.
type LatLong struct {
	Lat  float64
	Long float64
}

// IsZero reports whether m has no metadata to write.
func (m Meta) IsZero() bool {
	return m.Copyright == "" && m.Creator == "" && m.Title == "" && m.Description == "" && m.Date.IsZero() && m.LatLong == nil
}

// Validate reports whether m can be written.
func (m Meta) Validate() error {
	if ll := m.LatLong; ll != nil {
		if math.IsNaN(ll.Lat) || ll.Lat < -90 || ll.Lat > 90 {
			return fmt.Errorf("latitude %v out of range [-90, 90]", ll.Lat)
		}
		if math.IsNaN(ll.Long) || ll.Long < -180 || ll.Long > 180 {
			return fmt.Errorf("longitude %v out of range [-180, 180]", ll.Long)
		}
	}
	return nil
}

// WriteOptions contains the options for the Write function.
type WriteOptions struct {
	// R is the image to add metadata to.
	// It must not contain any EXIF or XMP metadata,
	// which is the case for images created by Go's (and Hugo's) image encoders.
	R io.Reader

	// W is where the image with metadata gets written.
	W io.Writer

	// The image format in R. See CanWrite for the supported formats.
	ImageFormat ImageFormat

	// The metadata to write. It is written as both EXIF and XMP,
	// except for Title, which has no common EXIF tag.
	Meta Meta
}

// CanWrite reports whether Write supports format f.
func CanWrite(f ImageFormat) bool {
	switch f {
	case JPEG, PNG, WebP, TIFF, AVIF:
		return true
	}
	return false
}

// Write copies the image in opts.R to opts.W with opts.Meta added.
// It returns ErrMetadataExists (wrapped) if the image already contains metadata
// and an InvalidFormatError if the image could not be parsed.
func Write(opts WriteOptions) error {
	if opts.R == nil || opts.W == nil {
		return errors.New("both a reader and a writer must be provided")
	}
	if !CanWrite(opts.ImageFormat) {
		return fmt.Errorf("writing metadata to %s is not supported", opts.ImageFormat)
	}
	if err := opts.Meta.Validate(); err != nil {
		return err
	}

	if opts.Meta.IsZero() {
		_, err := io.Copy(opts.W, opts.R)
		return err
	}

	b, err := io.ReadAll(opts.R)
	if err != nil {
		return err
	}

	base := &baseEncoder{
		sliceReader: newSliceReader(b, binary.BigEndian),
		w:           opts.W,
		meta:        opts.Meta,
	}

	var enc encoder

	switch opts.ImageFormat {
	case JPEG:
		enc = &imageEncoderJPEG{baseEncoder: base}
	case PNG:
		enc = &imageEncoderPNG{baseEncoder: base}
	case WebP:
		base.byteOrder = binary.LittleEndian
		enc = &imageEncoderWebP{baseEncoder: base}
	case TIFF:
		enc = &imageEncoderTIF{baseEncoder: base}
	case AVIF:
		enc = &imageEncoderAVIF{baseEncoder: base}
	}

	return enc.encode()
}

type encoder interface {
	encode() error
}

type baseEncoder struct {
	*sliceReader
	w    io.Writer
	meta Meta
}

func (e *baseEncoder) writeChunks(chunks ...[]byte) error {
	for _, c := range chunks {
		if len(c) == 0 {
			continue
		}
		if _, err := e.w.Write(c); err != nil {
			return err
		}
	}
	return nil
}

// sliceReader reads binary data from a byte slice.
// Reads past the end return zero values and set err.
type sliceReader struct {
	b         []byte
	pos       int
	byteOrder byteOrder
	err       error
}

func newSliceReader(b []byte, bo byteOrder) *sliceReader {
	return &sliceReader{b: b, byteOrder: bo}
}

func (s *sliceReader) remaining() int {
	return len(s.b) - s.pos
}

func (s *sliceReader) readBytes(n int) []byte {
	if s.err != nil || n < 0 || n > s.remaining() {
		if s.err == nil {
			s.err = newInvalidFormatErrorf("unexpected end of data")
		}
		return nil
	}
	p := s.b[s.pos : s.pos+n]
	s.pos += n
	return p
}

func (s *sliceReader) readFourCC() fourCC {
	var fcc fourCC
	if p := s.readBytes(4); p != nil {
		copy(fcc[:], p)
	}
	return fcc
}

func (s *sliceReader) read1() uint8 {
	if p := s.readBytes(1); p != nil {
		return p[0]
	}
	return 0
}

func (s *sliceReader) read2() uint16 {
	if p := s.readBytes(2); p != nil {
		return s.byteOrder.Uint16(p)
	}
	return 0
}

func (s *sliceReader) read4() uint32 {
	if p := s.readBytes(4); p != nil {
		return s.byteOrder.Uint32(p)
	}
	return 0
}

func (s *sliceReader) read8() uint64 {
	if p := s.readBytes(8); p != nil {
		return s.byteOrder.Uint64(p)
	}
	return 0
}

func (s *sliceReader) skip(n int) {
	s.readBytes(n)
}

func (s *sliceReader) seek(pos int) {
	if s.err != nil {
		return
	}
	if pos < 0 || pos > len(s.b) {
		s.err = newInvalidFormatErrorf("invalid seek offset %d", pos)
		return
	}
	s.pos = pos
}

func errMetadataExistsf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMetadataExists, fmt.Sprintf(format, args...))
}
