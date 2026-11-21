// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bep/imagemeta"
	qt "github.com/frankban/quicktest"
	"golang.org/x/image/tiff"
)

var testWriteMeta = imagemeta.Meta{
	Copyright:   "© 2026 Bjørn Erik Pedersen & <friends>",
	Creator:     "Bjørn Erik Pedersen",
	Title:       "Fjord på Vestlandet",
	Description: "Utsikt over fjorden. 日本語も。",
	Date:        time.Date(2025, 7, 14, 16, 32, 5, 0, time.FixedZone("", 2*3600)),
	LatLong:     &imagemeta.LatLong{Lat: 60.389444, Long: -5.33},
}

type writeTestImage struct {
	name   string
	format imagemeta.ImageFormat
	data   []byte
}

func writeTestImages(c *qt.C) []writeTestImage {
	img := image.NewNRGBA(image.Rect(0, 0, 24, 16))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	img2 := image.NewRGBA(img.Rect)
	for i := range img2.Pix {
		img2.Pix[i] = 255
	}

	encode := func(f func(*bytes.Buffer) error) []byte {
		var buf bytes.Buffer
		c.Assert(f(&buf), qt.IsNil)
		return buf.Bytes()
	}

	images := []writeTestImage{
		{"go.jpg", imagemeta.JPEG, encode(func(b *bytes.Buffer) error { return jpeg.Encode(b, img2, nil) })},
		{"go.png", imagemeta.PNG, encode(func(b *bytes.Buffer) error { return png.Encode(b, img) })},
		{"go-le.tif", imagemeta.TIFF, encode(func(b *bytes.Buffer) error {
			return tiff.Encode(b, img, &tiff.Options{Compression: tiff.Deflate, Predictor: true})
		})},
	}

	for _, name := range []string{"lossy.webp", "lossless-alpha.webp", "lossy-alpha.webp", "animated.webp", "opaque.avif", "alpha.avif"} {
		b, err := os.ReadFile(filepath.Join("testdata", "write", name))
		c.Assert(err, qt.IsNil)
		format := imagemeta.WebP
		if filepath.Ext(name) == ".avif" {
			format = imagemeta.AVIF
		}
		images = append(images, writeTestImage{name, format, b})
	}

	return images
}

func writeMeta(c *qt.C, format imagemeta.ImageFormat, data []byte, m imagemeta.Meta) ([]byte, error) {
	var buf bytes.Buffer
	err := imagemeta.Write(imagemeta.WriteOptions{R: bytes.NewReader(data), W: &buf, ImageFormat: format, Meta: m})
	return buf.Bytes(), err
}

func decodeAll(c *qt.C, format imagemeta.ImageFormat, data []byte) (imagemeta.Tags, imagemeta.DecodeResult) {
	var tags imagemeta.Tags
	res, err := imagemeta.Decode(imagemeta.Options{
		R:               bytes.NewReader(data),
		ImageFormat:     format,
		Sources:         imagemeta.EXIF | imagemeta.XMP | imagemeta.CONFIG,
		ShouldHandleTag: func(imagemeta.TagInfo) bool { return true },
		HandleTag: func(ti imagemeta.TagInfo) error {
			tags.Add(ti)
			return nil
		},
	})
	c.Assert(err, qt.IsNil)
	return tags, res
}

func TestWrite(t *testing.T) {
	c := qt.New(t)

	for _, img := range writeTestImages(c) {
		c.Run(img.name, func(c *qt.C) {
			_, before := decodeAll(c, img.format, img.data)

			out, err := writeMeta(c, img.format, img.data, testWriteMeta)
			c.Assert(err, qt.IsNil)

			tags, res := decodeAll(c, img.format, out)
			c.Assert(res.ImageConfig, qt.Equals, before.ImageConfig)

			exif, xmp := tags.EXIF(), tags.XMP()
			c.Assert(exif["Copyright"].Value, qt.Equals, testWriteMeta.Copyright)
			c.Assert(exif["Artist"].Value, qt.Equals, testWriteMeta.Creator)
			c.Assert(exif["ImageDescription"].Value, qt.Equals, testWriteMeta.Description)
			c.Assert(xmp["Rights"].Value, qt.Equals, testWriteMeta.Copyright)
			c.Assert(xmp["Creator"].Value, qt.Equals, testWriteMeta.Creator)
			c.Assert(xmp["Title"].Value, qt.Equals, testWriteMeta.Title)
			c.Assert(xmp["Description"].Value, qt.Equals, testWriteMeta.Description)

			d, err := tags.GetDateTime()
			c.Assert(err, qt.IsNil)
			c.Assert(d.Equal(testWriteMeta.Date), qt.IsTrue, qt.Commentf("%s", d))
			_, offset := d.Zone()
			c.Assert(offset, qt.Equals, 2*3600)

			lat, long, err := tags.GetLatLong()
			c.Assert(err, qt.IsNil)
			c.Assert(math.Abs(lat-testWriteMeta.LatLong.Lat) < 1e-6, qt.IsTrue, qt.Commentf("%v", lat))
			c.Assert(math.Abs(long-testWriteMeta.LatLong.Long) < 1e-6, qt.IsTrue, qt.Commentf("%v", long))

			// Still a valid image.
			switch img.format {
			case imagemeta.JPEG, imagemeta.PNG:
				_, _, err = image.Decode(bytes.NewReader(out))
				c.Assert(err, qt.IsNil)
			case imagemeta.TIFF:
				_, err = tiff.Decode(bytes.NewReader(out))
				c.Assert(err, qt.IsNil)
			}

			_, err = writeMeta(c, img.format, out, testWriteMeta)
			c.Assert(err, qt.ErrorIs, imagemeta.ErrMetadataExists)
		})
	}
}

func TestWriteXMPOnly(t *testing.T) {
	c := qt.New(t)

	for _, img := range writeTestImages(c) {
		c.Run(img.name, func(c *qt.C) {
			out, err := writeMeta(c, img.format, img.data, imagemeta.Meta{Title: "Tittel"})
			c.Assert(err, qt.IsNil)
			tags, _ := decodeAll(c, img.format, out)
			c.Assert(tags.XMP()["Title"].Value, qt.Equals, "Tittel")
			if img.format != imagemeta.TIFF {
				c.Assert(tags.EXIF(), qt.HasLen, 0)
			}
		})
	}
}

func TestWriteZeroMeta(t *testing.T) {
	c := qt.New(t)
	img := writeTestImages(c)[0]
	out, err := writeMeta(c, img.format, img.data, imagemeta.Meta{})
	c.Assert(err, qt.IsNil)
	c.Assert(out, qt.DeepEquals, img.data)
}

func TestWriteErrors(t *testing.T) {
	c := qt.New(t)
	img := writeTestImages(c)[0]

	_, err := writeMeta(c, img.format, img.data, imagemeta.Meta{LatLong: &imagemeta.LatLong{Lat: 91}})
	c.Assert(err, qt.ErrorMatches, `latitude 91 out of range.*`)
	_, err = writeMeta(c, img.format, img.data, imagemeta.Meta{LatLong: &imagemeta.LatLong{Long: math.NaN()}})
	c.Assert(err, qt.ErrorMatches, `longitude NaN out of range.*`)
	_, err = writeMeta(c, imagemeta.GIF, img.data, testWriteMeta)
	c.Assert(err, qt.ErrorMatches, `writing metadata to GIF is not supported`)
	c.Assert(imagemeta.CanWrite(imagemeta.GIF), qt.IsFalse)

	b, err := os.ReadFile(filepath.Join("testdata", "write", "animated.avif"))
	c.Assert(err, qt.IsNil)
	_, err = writeMeta(c, imagemeta.AVIF, b, testWriteMeta)
	c.Assert(err, qt.ErrorMatches, `.*AVIF image sequences is not supported`)

	for _, img := range writeTestImages(c) {
		for _, n := range []int{0, 1, 10, len(img.data) / 2} {
			_, err := writeMeta(c, img.format, img.data[:n], testWriteMeta)
			c.Assert(err, qt.Not(qt.IsNil), qt.Commentf("%s truncated to %d", img.name, n))
		}
	}
}

// TestWriteExiftool verifies the written files with ExifTool and, if installed, webpmux and avifdec.
func TestWriteExiftool(t *testing.T) {
	c := qt.New(t)
	if _, err := exec.LookPath("exiftool"); err != nil {
		c.Skip("exiftool not found")
	}

	dir := c.TempDir()
	for _, img := range writeTestImages(c) {
		c.Run(img.name, func(c *qt.C) {
			out, err := writeMeta(c, img.format, img.data, testWriteMeta)
			c.Assert(err, qt.IsNil)
			filename := filepath.Join(dir, img.name)
			c.Assert(os.WriteFile(filename, out, 0o644), qt.IsNil)

			b, err := exec.Command("exiftool", "-json", "-G", "-a", "-warning", "-error", "-c", "%.6f",
				"-EXIF:Copyright", "-EXIF:Artist", "-EXIF:ImageDescription", "-EXIF:DateTimeOriginal", "-EXIF:OffsetTimeOriginal",
				"-EXIF:GPSLatitude#", "-EXIF:GPSLongitude#", "-EXIF:GPSLongitudeRef",
				"-XMP:Rights", "-XMP:Creator", "-XMP:Title", "-XMP:Description", "-XMP:DateCreated",
				"-XMP:GPSLatitude#", "-XMP:GPSLongitude#",
				filename).CombinedOutput()
			c.Assert(err, qt.IsNil, qt.Commentf("%s", b))

			var res []map[string]any
			c.Assert(json.Unmarshal(b, &res), qt.IsNil, qt.Commentf("%s", b))
			m := res[0]
			c.Assert(m["ExifTool:Error"], qt.IsNil)
			c.Assert(m["ExifTool:Warning"], qt.IsNil, qt.Commentf("%s", b))
			c.Assert(m["EXIF:Copyright"], qt.Equals, testWriteMeta.Copyright)
			c.Assert(m["EXIF:Artist"], qt.Equals, testWriteMeta.Creator)
			c.Assert(m["EXIF:ImageDescription"], qt.Equals, testWriteMeta.Description)
			c.Assert(m["EXIF:DateTimeOriginal"], qt.Equals, "2025:07:14 16:32:05")
			c.Assert(m["EXIF:OffsetTimeOriginal"], qt.Equals, "+02:00")
			c.Assert(m["XMP:Rights"], qt.Equals, testWriteMeta.Copyright)
			c.Assert(m["XMP:Creator"], qt.Equals, testWriteMeta.Creator)
			c.Assert(m["XMP:Title"], qt.Equals, testWriteMeta.Title)
			c.Assert(m["XMP:Description"], qt.Equals, testWriteMeta.Description)
			c.Assert(m["XMP:DateCreated"], qt.Equals, "2025:07:14 16:32:05+02:00")
			for _, k := range []string{"EXIF:GPSLatitude", "XMP:GPSLatitude"} {
				c.Assert(math.Abs(m[k].(float64)-testWriteMeta.LatLong.Lat) < 1e-6, qt.IsTrue, qt.Commentf("%s: %v", k, m[k]))
			}
			decoders := map[imagemeta.ImageFormat][]string{
				imagemeta.WebP: {"webpmux", "-info", filename},
				imagemeta.AVIF: {"avifdec", filename, filename + ".png"},
			}
			if args, ok := decoders[img.format]; ok {
				if _, err := exec.LookPath(args[0]); err == nil {
					b, err := exec.Command(args[0], args[1:]...).CombinedOutput()
					c.Assert(err, qt.IsNil, qt.Commentf("%s", b))
				}
			}

			c.Assert(m["EXIF:GPSLongitudeRef"], qt.Equals, "West")
			c.Assert(math.Abs(m["EXIF:GPSLongitude"].(float64)+testWriteMeta.LatLong.Long) < 1e-6, qt.IsTrue)
			c.Assert(math.Abs(m["XMP:GPSLongitude"].(float64)-testWriteMeta.LatLong.Long) < 1e-6, qt.IsTrue)
		})
	}
}

func decodeSources(c *qt.C, format imagemeta.ImageFormat, data []byte, sources imagemeta.Source) imagemeta.Tags {
	var tags imagemeta.Tags
	_, err := imagemeta.Decode(imagemeta.Options{
		R:               bytes.NewReader(data),
		ImageFormat:     format,
		Sources:         sources,
		ShouldHandleTag: func(imagemeta.TagInfo) bool { return true },
		HandleTag: func(ti imagemeta.TagInfo) error {
			tags.Add(ti)
			return nil
		},
	})
	c.Assert(err, qt.IsNil)
	return tags
}

// TestWritePartialMeta verifies subsets of Meta, including GPS without Date (no ExifIFD).
func TestWritePartialMeta(t *testing.T) {
	c := qt.New(t)
	hasExiftool := true
	if _, err := exec.LookPath("exiftool"); err != nil {
		hasExiftool = false
	}

	c.Run("LatLong only", func(c *qt.C) {
		meta := imagemeta.Meta{LatLong: &imagemeta.LatLong{Lat: -33.8688, Long: 151.2093}}
		dir := c.TempDir()
		for _, img := range writeTestImages(c) {
			c.Run(img.name, func(c *qt.C) {
				out, err := writeMeta(c, img.format, img.data, meta)
				c.Assert(err, qt.IsNil)

				for _, src := range []imagemeta.Source{imagemeta.EXIF, imagemeta.XMP} {
					tags := decodeSources(c, img.format, out, src)
					lat, long, err := tags.GetLatLong()
					c.Assert(err, qt.IsNil)
					c.Assert(math.Abs(lat-meta.LatLong.Lat) < 1e-6, qt.IsTrue, qt.Commentf("%s lat=%v", src, lat))
					c.Assert(math.Abs(long-meta.LatLong.Long) < 1e-6, qt.IsTrue, qt.Commentf("%s long=%v", src, long))
				}

				if hasExiftool {
					fn := filepath.Join(dir, img.name)
					c.Assert(os.WriteFile(fn, out, 0o644), qt.IsNil)
					b, err := exec.Command("exiftool", "-json", "-G", "-warning", "-error", "-c", "%.6f",
						"-EXIF:GPSLatitude#", "-EXIF:GPSLatitudeRef", "-EXIF:GPSLongitude#",
						"-XMP:GPSLatitude#", "-XMP:GPSLongitude#", fn).CombinedOutput()
					c.Assert(err, qt.IsNil, qt.Commentf("%s", b))
					var res []map[string]any
					c.Assert(json.Unmarshal(b, &res), qt.IsNil, qt.Commentf("%s", b))
					m := res[0]
					c.Assert(m["ExifTool:Error"], qt.IsNil)
					c.Assert(m["ExifTool:Warning"], qt.IsNil, qt.Commentf("%s", b))
					c.Assert(m["EXIF:GPSLatitudeRef"], qt.Equals, "South")
					c.Assert(math.Abs(m["EXIF:GPSLatitude"].(float64)+meta.LatLong.Lat) < 1e-6, qt.IsTrue)
					c.Assert(math.Abs(m["XMP:GPSLatitude"].(float64)-meta.LatLong.Lat) < 1e-6, qt.IsTrue)
				}
			})
		}
	})

	c.Run("Date UTC and negative offset", func(c *qt.C) {
		img := writeTestImages(c)[0]
		for _, dt := range []time.Time{
			time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
			time.Date(2025, 1, 2, 3, 4, 5, 0, time.FixedZone("", -5*3600)),
		} {
			out, err := writeMeta(c, img.format, img.data, imagemeta.Meta{Date: dt})
			c.Assert(err, qt.IsNil)
			for _, src := range []imagemeta.Source{imagemeta.EXIF, imagemeta.XMP} {
				tags := decodeSources(c, img.format, out, src)
				got, err := tags.GetDateTime()
				c.Assert(err, qt.IsNil)
				c.Assert(got.Equal(dt), qt.IsTrue)
				_, wantOff := dt.Zone()
				_, gotOff := got.Zone()
				c.Assert(gotOff, qt.Equals, wantOff)
			}
		}
	})
}

func TestWriteBigEndianTIFFAndConflicts(t *testing.T) {
	c := qt.New(t)
	be := binary.BigEndian

	// Minimal valid big-endian 1x1 grayscale TIFF (ImageWidth=1, ImageLength=1, StripOffsets=8, StripByteCounts=1).
	var buf bytes.Buffer
	buf.WriteString("MM")
	binary.Write(&buf, be, uint16(42))
	binary.Write(&buf, be, uint32(10)) // IFD0 at offset 10 (even)
	buf.Write([]byte{0x7f, 0x00})      // 1 pixel + pad
	binary.Write(&buf, be, uint16(4))  // 4 entries
	writeEntry := func(tag, typ uint16, count, val uint32) {
		binary.Write(&buf, be, tag)
		binary.Write(&buf, be, typ)
		binary.Write(&buf, be, count)
		binary.Write(&buf, be, val)
	}
	writeEntry(0x0100, 4, 1, 1) // ImageWidth
	writeEntry(0x0101, 4, 1, 1) // ImageLength
	writeEntry(0x0111, 4, 1, 8) // StripOffsets
	writeEntry(0x0117, 4, 1, 1) // StripByteCounts
	binary.Write(&buf, be, uint32(0))

	out, err := writeMeta(c, imagemeta.TIFF, buf.Bytes(), testWriteMeta)
	c.Assert(err, qt.IsNil)
	tags, res := decodeAll(c, imagemeta.TIFF, out)
	c.Assert(res.ImageConfig.Width, qt.Equals, 1)
	c.Assert(res.ImageConfig.Height, qt.Equals, 1)
	c.Assert(tags.EXIF()["Copyright"].Value, qt.Equals, testWriteMeta.Copyright)
	c.Assert(tags.XMP()["Title"].Value, qt.Equals, testWriteMeta.Title)

	// Existing Copyright tag in TIFF must reject writing even XMP-only Title.
	var withCopy bytes.Buffer
	withCopy.WriteString("MM")
	binary.Write(&withCopy, be, uint16(42))
	binary.Write(&withCopy, be, uint32(8))
	binary.Write(&withCopy, be, uint16(1))
	binary.Write(&withCopy, be, uint16(0x8298)) // Copyright
	binary.Write(&withCopy, be, uint16(2))      // ASCII
	binary.Write(&withCopy, be, uint32(2))
	withCopy.Write([]byte{'A', 0, 0, 0})
	binary.Write(&withCopy, be, uint32(0))

	_, err = writeMeta(c, imagemeta.TIFF, withCopy.Bytes(), imagemeta.Meta{Title: "New"})
	c.Assert(err, qt.ErrorIs, imagemeta.ErrMetadataExists)
}

func TestWriteWebPOddChunkNoPad(t *testing.T) {
	c := qt.New(t)
	orig, err := os.ReadFile(filepath.Join("testdata", "write", "lossy.webp"))
	c.Assert(err, qt.IsNil)

	// Trim to create an odd-sized VP8 chunk without a trailing RIFF pad byte.
	le := binary.LittleEndian
	vp8Len := le.Uint32(orig[16:20])
	if vp8Len%2 == 0 {
		// Append 1 dummy byte to the VP8 payload so its length is odd and omit the RIFF pad byte.
		b := make([]byte, len(orig)+1)
		copy(b, orig)
		le.PutUint32(b[16:20], vp8Len+1)
		le.PutUint32(b[4:8], uint32(len(b)-8))
		orig = b
	} else if len(orig)%2 == 0 {
		orig = orig[:len(orig)-1]
		le.PutUint32(orig[4:8], uint32(len(orig)-8))
	}

	out, err := writeMeta(c, imagemeta.WebP, orig, testWriteMeta)
	c.Assert(err, qt.IsNil)
	tags, _ := decodeAll(c, imagemeta.WebP, out)
	c.Assert(tags.EXIF()["Copyright"].Value, qt.Equals, testWriteMeta.Copyright)
	c.Assert(tags.XMP()["Title"].Value, qt.Equals, testWriteMeta.Title)
}

func TestWriteAVIFEdgeCases(t *testing.T) {
	c := qt.New(t)
	orig, err := os.ReadFile(filepath.Join("testdata", "write", "opaque.avif"))
	c.Assert(err, qt.IsNil)

	// Parse top-level boxes of opaque.avif (ftyp, meta, mdat) and reorder to (ftyp, mdat, meta)
	// with meta.size = 0 (extends to EOF) and adjusted iloc offsets pointing into mdat before meta.
	be := binary.BigEndian
	readBox := func(b []byte, pos int) (typ string, end int) {
		sz := int(be.Uint32(b[pos:]))
		return string(b[pos+4 : pos+8]), pos + sz
	}
	t0, e0 := readBox(orig, 0)
	c.Assert(t0, qt.Equals, "ftyp")
	t1, e1 := readBox(orig, e0)
	c.Assert(t1, qt.Equals, "meta")
	t2, e2 := readBox(orig, e1)
	c.Assert(t2, qt.Equals, "mdat")
	c.Assert(e2, qt.Equals, len(orig))

	ftyp := orig[:e0]
	metaBox := append([]byte(nil), orig[e0:e1]...)
	mdat := orig[e1:e2]

	// Locate iloc inside metaBox and shift its item offset backwards by len(metaBox)
	// since mdat now starts at len(ftyp) instead of len(ftyp)+len(metaBox).
	ilocOff := bytes.Index(metaBox, []byte("iloc"))
	c.Assert(ilocOff > 4, qt.IsTrue)
	// opaque.avif uses iloc v0 with 4-byte offset/length, 0-byte base, 1 item, 1 extent.
	// iloc header: size(4)+"iloc"(4)+vflags(4)+sizes(2)+count(2)+id(2)+dataRef(2)+extCount(2) = 22 bytes to offset.
	offPos := ilocOff - 4 + 22
	oldOff := be.Uint32(metaBox[offPos:])
	be.PutUint32(metaBox[offPos:], oldOff-uint32(len(metaBox)))

	// Set meta size to 0 (extends to EOF).
	be.PutUint32(metaBox[0:4], 0)
	reordered := append(append(append([]byte(nil), ftyp...), mdat...), metaBox...)

	out, err := writeMeta(c, imagemeta.AVIF, reordered, testWriteMeta)
	c.Assert(err, qt.IsNil)
	tags, _ := decodeAll(c, imagemeta.AVIF, out)
	c.Assert(tags.EXIF()["Copyright"].Value, qt.Equals, testWriteMeta.Copyright)
	c.Assert(tags.XMP()["Title"].Value, qt.Equals, testWriteMeta.Title)

	if _, err := exec.LookPath("avifdec"); err == nil {
		dir := c.TempDir()
		fn := filepath.Join(dir, "meta-eof.avif")
		c.Assert(os.WriteFile(fn, out, 0o644), qt.IsNil)
		b, err := exec.Command("avifdec", fn, fn+".png").CombinedOutput()
		c.Assert(err, qt.IsNil, qt.Commentf("%s", b))
	}

	// Crafted iloc with zero-sized fields and huge extentCount must be rejected without OOM.
	c.Run("iloc zero-size extent amplification", func(c *qt.C) {
		var ilocPayload bytes.Buffer
		ilocPayload.Write([]byte{2, 0, 0, 0}) // version 2, flags 0
		ilocPayload.Write([]byte{0x00, 0x00}) // off=0, len=0, base=0, idx=0
		binary.Write(&ilocPayload, be, uint32(10))
		for i := range 10 {
			binary.Write(&ilocPayload, be, uint32(i+1)) // id
			binary.Write(&ilocPayload, be, uint16(0))   // method
			binary.Write(&ilocPayload, be, uint16(0))   // dataRef
			binary.Write(&ilocPayload, be, uint16(0xffff))
		}
		mkBox := func(typ string, payload []byte) []byte {
			b := be.AppendUint32(nil, uint32(8+len(payload)))
			b = append(b, typ...)
			return append(b, payload...)
		}
		meta := mkBox("meta", append([]byte{0, 0, 0, 0}, mkBox("iloc", ilocPayload.Bytes())...))
		avif := append(mkBox("ftyp", []byte("avif\x00\x00\x00\x00avif")), meta...)

		_, err := writeMeta(c, imagemeta.AVIF, avif, testWriteMeta)
		c.Assert(imagemeta.IsInvalidFormat(err), qt.IsTrue, qt.Commentf("err=%v", err))
	})
}

func TestWriteMalformedInputs(t *testing.T) {
	c := qt.New(t)
	imgs := writeTestImages(c)

	// Truncated JPEGs (missing SOS/EOI or invalid marker 0x00).
	for _, b := range [][]byte{
		{0xFF, 0xD8, 0xFF, 0xE0, 0x00},
		{0xFF, 0xD8, 0xFF, 0xFF},
		{0xFF, 0xD8, 0xFF, 0x00, 0x00, 0x02},
		{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02}, // Valid APP0 but no SOS/EOI.
	} {
		_, err := writeMeta(c, imagemeta.JPEG, b, testWriteMeta)
		c.Assert(imagemeta.IsInvalidFormat(err), qt.IsTrue, qt.Commentf("jpeg %x: %v", b, err))
	}

	// Oversized JPEG metadata is not an InvalidFormatError.
	_, err := writeMeta(c, imagemeta.JPEG, imgs[0].data, imagemeta.Meta{Description: strings.Repeat("a", 70000)})
	c.Assert(err, qt.Not(qt.IsNil))
	c.Assert(imagemeta.IsInvalidFormat(err), qt.IsFalse)

	// Truncated PNG (signature + IHDR only, missing IEND).
	pngBytes := imgs[1].data
	ihdrEnd := 8 + 12 + int(binary.BigEndian.Uint32(pngBytes[8:12]))
	_, err = writeMeta(c, imagemeta.PNG, pngBytes[:ihdrEnd], testWriteMeta)
	c.Assert(imagemeta.IsInvalidFormat(err), qt.IsTrue)

	// Malformed TIFF with IFD0 offset < 8.
	_, err = writeMeta(c, imagemeta.TIFF, []byte{'I', 'I', 42, 0, 0, 0, 0, 0, 0, 0}, testWriteMeta)
	c.Assert(imagemeta.IsInvalidFormat(err), qt.IsTrue)

	// Malformed WebP inputs (short VP8X, short VP8L, zero-dim VP8, truncated chunk).
	mkRIFF := func(chunks []byte) []byte {
		b := append([]byte("RIFF"), 0, 0, 0, 0)
		b = append(b, "WEBP"...)
		b = append(b, chunks...)
		binary.LittleEndian.PutUint32(b[4:8], uint32(len(b)-8))
		return b
	}
	for _, b := range [][]byte{
		mkRIFF([]byte{'V', 'P', '8', 'X', 0, 0, 0, 0, 'V', 'P', '8', ' ', 0, 0, 0, 0}),
		mkRIFF([]byte{'V', 'P', '8', 'X', 5, 0, 0, 0, 0, 0, 0, 0, 0, 0}),
		mkRIFF([]byte{'V', 'P', '8', 'L', 2, 0, 0, 0, 0x2f, 0}),
		mkRIFF([]byte{'V', 'P', '8', ' ', 10, 0, 0, 0, 0, 0, 0, 0x9d, 0x01, 0x2a, 0, 0, 1, 0}),
		mkRIFF([]byte{'V', 'P', '8', ' ', 100, 0, 0, 0, 0, 0, 0, 0x9d, 0x01, 0x2a, 1, 0, 1, 0}),
	} {
		_, err := writeMeta(c, imagemeta.WebP, b, testWriteMeta)
		c.Assert(imagemeta.IsInvalidFormat(err), qt.IsTrue, qt.Commentf("webp %x: %v", b, err))
	}

	// Malformed AVIF: short iref box (payload < 4 bytes).
	mkBox := func(typ string, payload []byte) []byte {
		b := binary.BigEndian.AppendUint32(nil, uint32(8+len(payload)))
		b = append(b, typ...)
		return append(b, payload...)
	}
	shortIrefAVIF := append(
		mkBox("ftyp", []byte("avif\x00\x00\x00\x00avif")),
		mkBox("meta", append([]byte{0, 0, 0, 0}, mkBox("iref", []byte{0})...))...,
	)
	_, err = writeMeta(c, imagemeta.AVIF, shortIrefAVIF, testWriteMeta)
	c.Assert(imagemeta.IsInvalidFormat(err), qt.IsTrue)
}

func TestDecodePNGCompressedXMPAndInvalidOffsetTime(t *testing.T) {
	c := qt.New(t)

	// Build a minimal PNG with a compressed iTXt XMP chunk (valid and invalid compression method).
	makePNGWithiTXt := func(compMethod byte) []byte {
		var zbuf bytes.Buffer
		zw := zlib.NewWriter(&zbuf)
		zw.Write([]byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title><rdf:Alt><rdf:li xml:lang="x-default">Compressed</rdf:li></rdf:Alt></dc:title></rdf:Description></rdf:RDF></x:xmpmeta>`))
		zw.Close()

		payload := append([]byte("XML:com.adobe.xmp\x00"), 1, compMethod, 0, 0)
		payload = append(payload, zbuf.Bytes()...)

		chunk := func(typ string, data []byte) []byte {
			b := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
			start := len(b)
			b = append(b, typ...)
			b = append(b, data...)
			return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(b[start:]))
		}
		var out []byte
		out = append(out, "\x89PNG\r\n\x1a\n"...)
		out = append(out, chunk("IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, 8, 2, 0, 0, 0})...)
		out = append(out, chunk("iTXt", payload)...)
		out = append(out, chunk("IEND", nil)...)
		return out
	}

	tags := decodeSources(c, imagemeta.PNG, makePNGWithiTXt(0), imagemeta.XMP)
	c.Assert(tags.XMP()["Title"].Value, qt.Equals, "Compressed")

	_, err := imagemeta.Decode(imagemeta.Options{
		R:           bytes.NewReader(makePNGWithiTXt(1)),
		ImageFormat: imagemeta.PNG,
		Sources:     imagemeta.XMP,
	})
	c.Assert(imagemeta.IsInvalidFormat(err), qt.IsTrue)

	// EXIF placeholder OffsetTimeOriginal ("+  :  ") must not break GetDateTime.
	var tTags imagemeta.Tags
	tTags.Add(imagemeta.TagInfo{Source: imagemeta.EXIF, Tag: "DateTimeOriginal", Value: "2025:07:14 16:32:05"})
	tTags.Add(imagemeta.TagInfo{Source: imagemeta.EXIF, Tag: "OffsetTimeOriginal", Value: "+  :  "})
	d, err := tTags.GetDateTime()
	c.Assert(err, qt.IsNil)
	c.Assert(d.Year(), qt.Equals, 2025)
	c.Assert(d.Month(), qt.Equals, time.July)
	c.Assert(d.Day(), qt.Equals, 14)
}
