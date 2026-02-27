// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bep/imagemeta"
	"github.com/rwcarlsen/goexif/exif"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
)

func TestDecodeAllImageFormats(t *testing.T) {
	c := qt.New(t)

	for _, imageFormat := range []imagemeta.ImageFormat{imagemeta.JPEG, imagemeta.TIFF, imagemeta.PNG, imagemeta.WebP} {
		c.Run(fmt.Sprintf("%v", imageFormat), func(c *qt.C) {
			img, close := getSunrise(c, imageFormat)
			c.Cleanup(close)

			var tags imagemeta.Tags
			handleTag := func(ti imagemeta.TagInfo) error {
				tags.Add(ti)
				return nil
			}

			_, err := imagemeta.Decode(imagemeta.Options{R: img, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf})
			c.Assert(err, qt.IsNil)

			allTags := tags.All()
			exifTags := tags.EXIF()

			c.Assert(len(allTags), qt.Not(qt.Equals), 0)

			c.Assert(allTags["Headline"].Value, qt.Equals, "Sunrise in Spain")
			c.Assert(allTags["Copyright"].Value, qt.Equals, "Bjørn Erik Pedersen")
			c.Assert(exifTags["Orientation"].Value, qt.Equals, uint16(1))
			et, _ := imagemeta.NewRat[uint32](1, 200)
			fl, _ := imagemeta.NewRat[uint32](21, 1)
			c.Assert(exifTags["ExposureTime"].Value, eq, et)
			c.Assert(exifTags["FocalLength"].Value, eq, fl)
		})
	}
}

func TestDecodeHEIF(t *testing.T) {
	c := qt.New(t)

	// Test iphone.heic (HEIF/HEIC with EXIF, no XMP for these tags).
	_, tags, err := extractTags(t, "iphone.heic", imagemeta.EXIF|imagemeta.XMP)
	c.Assert(err, qt.IsNil)
	c.Assert(tags.EXIF()["Make"].Value, qt.Equals, "Apple")
	c.Assert(tags.EXIF()["Model"].Value, qt.Equals, "iPhone 15 Pro")
	c.Assert(tags.EXIF()["Orientation"].Value, qt.Equals, uint16(1))

	// Test sony.heif (HEIF with EXIF, little-endian byte order).
	_, tags, err = extractTags(t, "sony.heif", imagemeta.EXIF|imagemeta.XMP)
	c.Assert(err, qt.IsNil)
	c.Assert(tags.EXIF()["Make"].Value, qt.Equals, "SONY")
	c.Assert(tags.EXIF()["Model"].Value, qt.Equals, "ILCE-6700")
}

func TestDecodeHEIFConfig(t *testing.T) {
	c := qt.New(t)

	f, err := os.Open(filepath.Join("testdata", "images", "iphone.heic"))
	c.Assert(err, qt.IsNil)
	defer f.Close()

	res, err := imagemeta.Decode(imagemeta.Options{
		R:           f,
		ImageFormat: imagemeta.HEIF,
		Sources:     imagemeta.CONFIG,
		HandleTag:   func(ti imagemeta.TagInfo) error { return nil },
		Warnf:       panicWarnf,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(res.ImageConfig.Width, qt.Equals, 5712)
	c.Assert(res.ImageConfig.Height, qt.Equals, 4284)

	// Sony A6700: coded 6192x4128, irot angle=3 (270° CCW) swaps to 4128x6192.
	f2, err := os.Open(filepath.Join("testdata", "images", "sony.heif"))
	c.Assert(err, qt.IsNil)
	defer f2.Close()

	res2, err := imagemeta.Decode(imagemeta.Options{
		R:           f2,
		ImageFormat: imagemeta.HEIF,
		Sources:     imagemeta.CONFIG,
		HandleTag:   func(ti imagemeta.TagInfo) error { return nil },
		Warnf:       panicWarnf,
	})
	c.Assert(err, qt.IsNil)
	c.Assert(res2.ImageConfig.Width, qt.Equals, 4128)
	c.Assert(res2.ImageConfig.Height, qt.Equals, 6192)
}

func TestDecodeRAW(t *testing.T) {
	c := qt.New(t)

	// DNG (Canon PowerShot SD450)
	_, tags, err := extractTags(t, "sample.dng", imagemeta.EXIF)
	c.Assert(err, qt.IsNil)
	c.Assert(tags.EXIF()["Make"].Value, qt.Equals, "Canon")
	c.Assert(tags.EXIF()["Model"].Value, qt.Equals, "Canon PowerShot SD450")

	// CR2 (Canon PowerShot S90)
	_, tags, err = extractTags(t, "sample.cr2", imagemeta.EXIF)
	c.Assert(err, qt.IsNil)
	c.Assert(tags.EXIF()["Make"].Value, qt.Equals, "Canon")
	c.Assert(tags.EXIF()["Model"].Value, qt.Equals, "Canon PowerShot S90")

	// NEF (Nikon D1)
	_, tags, err = extractTags(t, "sample.nef", imagemeta.EXIF)
	c.Assert(err, qt.IsNil)
	c.Assert(tags.EXIF()["Make"].Value, qt.Equals, "NIKON CORPORATION")
	c.Assert(tags.EXIF()["Model"].Value, qt.Equals, "NIKON D1")

	// ARW (Sony DSLR-A330)
	_, tags, err = extractTags(t, "sample.arw", imagemeta.EXIF)
	c.Assert(err, qt.IsNil)
	c.Assert(tags.EXIF()["Make"].Value, qt.Equals, "SONY")
	c.Assert(tags.EXIF()["Model"].Value, qt.Equals, "DSLR-A330")

	// PEF (Pentax K-3 II)
	_, tags, err = extractTags(t, "bep/jølstravatnet.pef", imagemeta.EXIF)
	c.Assert(err, qt.IsNil)
	c.Assert(tags.EXIF()["Make"].Value, qt.Equals, "RICOH IMAGING COMPANY, LTD.")
	c.Assert(tags.EXIF()["Model"].Value, qt.Equals, "PENTAX K-3 II")
}

func TestDecodeRAWConfig(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		filename string
		format   imagemeta.ImageFormat
		width    int
		height   int
	}{
		{"sample.dng", imagemeta.DNG, 2592, 1944}, // DefaultCropSize
		{"sample.cr2", imagemeta.CR2, 3648, 2736}, // ExifIFD dimensions
		{"sample.nef", imagemeta.NEF, 2012, 1324}, // SubIFD dimensions
		{"sample.arw", imagemeta.ARW, 3880, 2600},              // ExifIFD dimensions
		{"bep/jølstravatnet.pef", imagemeta.PEF, 6080, 4032}, // IFD0 dimensions
	}

	for _, tt := range tests {
		c.Run(tt.filename, func(c *qt.C) {
			f, err := os.Open(filepath.Join("testdata", "images", tt.filename))
			c.Assert(err, qt.IsNil)
			defer f.Close()

			res, err := imagemeta.Decode(imagemeta.Options{
				R:           f,
				ImageFormat: tt.format,
				Sources:     imagemeta.CONFIG,
				HandleTag:   func(ti imagemeta.TagInfo) error { return nil },
				Warnf:       panicWarnf,
			})
			c.Assert(err, qt.IsNil)
			c.Assert(res.ImageConfig.Width, qt.Equals, tt.width)
			c.Assert(res.ImageConfig.Height, qt.Equals, tt.height)
		})
	}
}

func TestDecodeWebP(t *testing.T) {
	c := qt.New(t)
	_, tags, err := extractTags(t, "bep/sunrise.webp", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP)
	c.Assert(err, qt.IsNil)

	c.Assert(tags.EXIF()["Copyright"].Value, qt.Equals, "Bjørn Erik Pedersen")
	c.Assert(tags.EXIF()["ApertureValue"].Value, eq, 5.6)
	c.Assert(tags.XMP()["CreatorTool"].Value, qt.Equals, "Adobe Photoshop Lightroom Classic 12.4 (Macintosh)")
	// No IPTC in this file, Exiftool stores the IPTC fields in XMP.
	c.Assert(tags.XMP()["City"].Value, qt.Equals, "Benalmádena")
}

func TestDecodeAVIF(t *testing.T) {
	c := qt.New(t)
	_, tags, err := extractTags(t, "bep/sunset.avif", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP)
	c.Assert(err, qt.IsNil)

	c.Assert(tags.EXIF()["Artist"].Value, qt.Equals, "bjorn.erik.pedersen@gmail.com")
	c.Assert(tags.EXIF()["ApertureValue"].Value, eq, 5.6)
	c.Assert(tags.XMP()["CreatorTool"].Value, qt.Equals, "Adobe Photoshop Lightroom 6.12 (Macintosh)")
	c.Assert(tags.XMP()["City"].Value, qt.Equals, "Benalmádena")
}

func TestDecodeXmpChildElements(t *testing.T) {
	c := qt.New(t)
	_, tags, err := extractTags(t, "mushroom.jpg", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP)
	c.Assert(err, qt.IsNil)

	c.Assert(tags.XMP()["Subject"].Value, qt.DeepEquals, []string{"autumn", "closeup", "forest", "forestPhotography", "mushroom", "nature", "naturePhotography", "photo", "photography"})
	c.Assert(tags.XMP()["Rights"].Value, qt.Equals, "Creative Commons Attribution-ShareAlike (CC BY-SA)")
	c.Assert(tags.XMP()["Creator"].Value, qt.Equals, "Lukas Nagel")
	c.Assert(tags.XMP()["Publisher"].Value, qt.Equals, "LNA-DEV")
}

// For development.
func TestDecodeAdhoc(t *testing.T) {
	t.Skip("used in development")
	extractTags(t, "/Users/bep/dump/summer.webp", imagemeta.EXIF)
}

func TestDecodeJPEG(t *testing.T) {
	c := qt.New(t)

	shouldInclude := func(ti imagemeta.TagInfo) bool {
		return true
	}

	_, tags, err := extractTagsWithFilter(t, "bep/sunrise.jpg", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP, shouldInclude)
	c.Assert(err, qt.IsNil)

	c.Assert(tags.EXIF()["Copyright"].Value, qt.Equals, "Bjørn Erik Pedersen")
	c.Assert(tags.EXIF()["ApertureValue"].Value, eq, 5.6)
	c.Assert(tags.EXIF()["ThumbnailOffset"].Value, eq, uint32(1338))
	c.Assert(tags.XMP()["CreatorTool"].Value, qt.Equals, "Adobe Photoshop Lightroom Classic 12.4 (Macintosh)")
	c.Assert(tags.IPTC()["City"].Value, qt.Equals, "Benalmádena")
}

func TestDecodePNG(t *testing.T) {
	c := qt.New(t)

	shouldInclude := func(ti imagemeta.TagInfo) bool {
		return true
	}

	_, tags, err := extractTagsWithFilter(t, "bep/sunrise.png", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP, shouldInclude)
	c.Assert(err, qt.IsNil)

	c.Assert(len(tags.EXIF()), qt.Equals, 61)
	c.Assert(len(tags.IPTC()), qt.Equals, 14)

	c.Assert(tags.EXIF()["Copyright"].Value, qt.Equals, "Bjørn Erik Pedersen")
	c.Assert(tags.EXIF()["ApertureValue"].Value, eq, 5.6)
	c.Assert(tags.EXIF()["ThumbnailOffset"].Value, eq, uint32(1326))
	c.Assert(tags.IPTC()["City"].Value, qt.Equals, "Benalmádena")
}

func TestThumbnailOffset(t *testing.T) {
	c := qt.New(t)

	shouldHandle := func(ti imagemeta.TagInfo) bool {
		// Only include the thumbnail tags.
		return ti.Namespace == "IFD1"
	}

	offset := func(filename string) uint32 {
		_, tags, err := extractTagsWithFilter(t, filename, imagemeta.EXIF, shouldHandle)
		c.Assert(err, qt.IsNil)
		return tags.EXIF()["ThumbnailOffset"].Value.(uint32)
	}

	c.Assert(offset("bep/sunrise.webp"), eq, uint32(64160))
	c.Assert(offset("bep/sunrise.png"), eq, uint32(1326))
	c.Assert(offset("bep/sunrise.jpg"), eq, uint32(1338))
	c.Assert(offset("goexif/has-lens-info.jpg"), eq, uint32(1274))
}

func TestDecodeTIFF(t *testing.T) {
	c := qt.New(t)

	_, tags, err := extractTags(t, "bep/sunrise.tif", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP)
	c.Assert(err, qt.IsNil)

	c.Assert(len(tags.EXIF()), qt.Equals, 76)
	c.Assert(len(tags.XMP()), qt.Equals, 149)
	c.Assert(len(tags.IPTC()), qt.Equals, 14)

	c.Assert(tags.EXIF()["ShutterSpeedValue"].Value, eq, 0.005000000)
	c.Assert(tags.XMP()["CreatorTool"].Value, qt.Equals, "Adobe Photoshop Lightroom Classic 12.4 (Macintosh)")
	c.Assert(tags.IPTC()["Headline"].Value, eq, "Sunrise in Spain")
}

func TestDecodeCorrupt(t *testing.T) {
	c := qt.New(t)

	files, err := filepath.Glob(filepath.Join("testdata", "images", "corrupt", "*.*"))
	c.Assert(err, qt.IsNil)
	c.Assert(files, qt.HasLen, 3)

	for _, file := range files {
		img, err := os.Open(file)
		c.Assert(err, qt.IsNil)
		ext := filepath.Ext(file)
		format := extToFormat(ext)
		if format == -1 {
			continue
		}
		handleTag := func(ti imagemeta.TagInfo) error {
			return nil
		}
		_, err = imagemeta.Decode(imagemeta.Options{R: img, ImageFormat: format, HandleTag: handleTag, Warnf: panicWarnf})

		if !imagemeta.IsInvalidFormat(err) {
			c.Assert(err, qt.ErrorMatches, "UserComment: expected \\[\\]uint8, got string", qt.Commentf("file: %s", file))
		}

		img.Close()
	}
}

func TestDecodeUserCommentWithInvalidEncoding(t *testing.T) {
	c := qt.New(t)

	img, err := os.Open(filepath.Join("testdata", "images", "invalid-encoding-usercomment.jpg"))
	c.Assert(err, qt.IsNil)
	defer img.Close()

	var userComment any
	handleTag := func(ti imagemeta.TagInfo) error {
		if ti.Tag == "UserComment" {
			userComment = ti.Value
		}
		return nil
	}

	warnf := func(format string, args ...any) {
	}

	_, err = imagemeta.Decode(imagemeta.Options{R: img, ImageFormat: imagemeta.JPEG, HandleTag: handleTag, Sources: imagemeta.EXIF, Warnf: warnf})
	c.Assert(err, qt.IsNil)

	// Expect user comment to be decoded
	c.Assert(userComment, qt.IsNotNil)
	c.Assert(userComment, eq, "UserComment")
}

func TestDecodeCustomXMPHandler(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c, imagemeta.WebP)
	c.Cleanup(close)

	var xml string
	_, err := imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.WebP,
			HandleXMP: func(r io.Reader) error {
				b, err := io.ReadAll(r)
				xml = string(b)
				return err
			},
			Sources: imagemeta.XMP,
			Warnf:   panicWarnf,
		},
	)

	c.Assert(err, qt.IsNil)
	c.Assert(xml, qt.Contains, "Sunrise in Spain")
}

func TestDecodeCustomXMPHandlerShortRead(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c, imagemeta.WebP)
	c.Cleanup(close)

	_, err := imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.WebP,
			HandleXMP: func(r io.Reader) error {
				return nil
			},
			Sources: imagemeta.XMP,
			Warnf:   panicWarnf,
		},
	)

	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "expected EOF after XMP")
}

func TestDecodeShouldHandleTagEXIF(t *testing.T) {
	c := qt.New(t)

	const numTagsTotal = 64

	for range 30 {
		img, close := getSunrise(c, imagemeta.JPEG)
		c.Cleanup(close)

		var added int

		handleTag := func(ti imagemeta.TagInfo) error {
			added++
			return nil
		}

		// Drop a random tag.
		drop := rand.Intn(numTagsTotal - 1)
		counter := 0
		shouldHandle := func(ti imagemeta.TagInfo) bool {
			if ti.Tag == "ExifOffset" {
				t.Fatalf("IFD pointers should not be passed to ShouldHandleTag")
			}
			b := counter != drop
			counter++
			return b
		}

		_, err := imagemeta.Decode(
			imagemeta.Options{
				R:               img,
				ImageFormat:     imagemeta.JPEG,
				Sources:         imagemeta.EXIF,
				HandleTag:       handleTag,
				ShouldHandleTag: shouldHandle,
				Warnf:           panicWarnf,
			},
		)

		c.Assert(err, qt.IsNil)
		c.Assert(added, qt.Equals, numTagsTotal-1)

		img.Seek(0, 0)

	}
}

func TestDecodeIPTCReference(t *testing.T) {
	c := qt.New(t)
	const filename = "IPTC-PhotometadataRef-Std2021.1.jpg"

	img, err := os.Open(filepath.Join("testdata", "images", filename))
	c.Assert(err, qt.IsNil)

	c.Cleanup(func() {
		c.Assert(img.Close(), qt.IsNil)
	})

	var tags imagemeta.Tags
	handleTag := func(ti imagemeta.TagInfo) error {
		if tags.Has(ti) {
			c.Fatalf("duplicate tag: %s", ti.Tag)
		}
		c.Assert(ti.Tag, qt.Not(qt.Contains), "Unknown")
		tags.Add(ti)
		return nil
	}

	_, err = imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.JPEG,
			HandleTag:   handleTag,
			Sources:     imagemeta.IPTC,
			Warnf:       panicWarnf,
		},
	)
	c.Assert(err, qt.IsNil)

	c.Assert(len(tags.IPTC()), qt.Equals, 22)
	// These hyphens looks odd, but it's how Exiftool has defined it.
	c.Assert(tags.IPTC()["By-line"].Value, qt.Equals, "Creator1 (ref2021.1)")
	c.Assert(tags.IPTC()["By-lineTitle"].Value, qt.Equals, "Creator's Job Title  (ref2021.1)")
	c.Assert(tags.IPTC()["DateCreated"].Value, qt.Equals, "2021:10:20")
	c.Assert(tags.IPTC()["Keywords"].Value, qt.DeepEquals, []string{"Keyword1ref2021.1", "Keyword2ref2021.1", "Keyword3ref2021.1"})
}

func TestDecodeIPTCReferenceGolden(t *testing.T) {
	compareWithExiftoolOutput(t, "IPTC-PhotometadataRef-Std2021.1.jpg", imagemeta.IPTC)
}

func TestDecodeNamespace(t *testing.T) {
	c := qt.New(t)

	shouldInclude := func(ti imagemeta.TagInfo) bool {
		return true
	}

	_, tags, err := extractTagsWithFilter(t, "bep/sunrise.jpg", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP, shouldInclude)
	c.Assert(err, qt.IsNil)

	c.Assert(tags.EXIF()["Artist"].Namespace, qt.Equals, "IFD0")
	c.Assert(tags.EXIF()["GPSLatitude"].Namespace, qt.Equals, "IFD0/GPSInfoIFD")
	c.Assert(tags.EXIF()["Compression"].Namespace, qt.Equals, "IFD1")
	c.Assert(tags.IPTC()["City"].Namespace, qt.Equals, "IPTCApplication")
	c.Assert(tags.XMP()["AlreadyApplied"].Namespace, qt.Equals, "http://ns.adobe.com/camera-raw-settings/1.0/")
}

func TestDecodeEXIFOrientationOnly(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c, imagemeta.JPEG)
	c.Cleanup(close)

	var tags imagemeta.Tags
	handleTag := func(ti imagemeta.TagInfo) error {
		if ti.Tag == "Orientation" {
			tags.Add(ti)
			return imagemeta.ErrStopWalking
		}
		return nil
	}

	_, err := imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.JPEG,
			HandleTag:   handleTag,
			Sources:     imagemeta.EXIF,
			Warnf:       panicWarnf,
		},
	)

	c.Assert(err, qt.IsNil)
	c.Assert(tags.EXIF()["Orientation"].Value, qt.Equals, uint16(1))
	c.Assert(len(tags.EXIF()), qt.Equals, 1)
}

func TestDecodeIPTCOrientationOnly(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c, imagemeta.JPEG)
	c.Cleanup(close)

	var tags imagemeta.Tags
	handleTag := func(ti imagemeta.TagInfo) error {
		if ti.Tag == "Category" {
			tags.Add(ti)
			return imagemeta.ErrStopWalking
		}
		return nil
	}

	_, err := imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.JPEG,
			HandleTag:   handleTag,
			Sources:     imagemeta.IPTC,
			Warnf:       panicWarnf,
		},
	)

	c.Assert(err, qt.IsNil)
	c.Assert(tags.IPTC()["Category"].Value, qt.Equals, "Sun")
	c.Assert(len(tags.IPTC()), qt.Equals, 1)
}

func TestDecodeLargeExifTimeout(t *testing.T) {
	c := qt.New(t)

	withOpts := func(opts *imagemeta.Options) {
		opts.Timeout = time.Duration(500 * time.Millisecond)

		// Set the limits to something high to make sure we time out.
		opts.LimitNumTags = 1000000
		opts.LimitTagSize = 10000000
	}
	_, _, err := extractTags(t, "largeexif.png", imagemeta.EXIF, withOpts)
	c.Assert(err, qt.ErrorMatches, "timed out after 500ms")
}

func TestDecodeXMPJPG(t *testing.T) {
	c := qt.New(t)

	_, tags, err := extractTags(t, "bep/sunrise.jpg", imagemeta.XMP)
	c.Assert(err, qt.IsNil)

	c.Assert(len(tags.EXIF()) == 0, qt.IsTrue)
	c.Assert(len(tags.IPTC()) == 0, qt.IsTrue)
	c.Assert(len(tags.XMP()) > 0, qt.IsTrue)
}

func TestDecodeErrors(t *testing.T) {
	c := qt.New(t)

	decode := func(opts imagemeta.Options) error {
		_, err := imagemeta.Decode(opts)
		return err
	}

	c.Assert(decode(imagemeta.Options{}), qt.ErrorMatches, "no reader provided")
	c.Assert(decode(imagemeta.Options{R: strings.NewReader("foo")}), qt.ErrorMatches, "no image format provided.*")
	c.Assert(decode(imagemeta.Options{R: strings.NewReader("foo"), ImageFormat: imagemeta.ImageFormat(1234)}), qt.ErrorMatches, "unsupported image format")
}

func TestGoldenEXIFHugoIssue12669(t *testing.T) {
	compareWithExiftoolOutput(t, "hugo-issue-12669.jpg", imagemeta.EXIF)
}

func TestGoldenEXIFIssue34(t *testing.T) {
	compareWithExiftoolOutput(t, "outofbounds-issue-34.jpg", imagemeta.EXIF)
}

func TestGoldenEXIF(t *testing.T) {
	withGolden(t, imagemeta.EXIF)
}

func TestGoldenIPTC(t *testing.T) {
	withGolden(t, imagemeta.IPTC)
}

func TestGoldenEXIFAndIPTC(t *testing.T) {
	withGolden(t, imagemeta.EXIF|imagemeta.IPTC)
}

func TestGoldenConfig(t *testing.T) {
	withGolden(t, imagemeta.CONFIG)
}

func TestGoldenEXIFAndIPTCAndConfig(t *testing.T) {
	withGolden(t, imagemeta.EXIF|imagemeta.IPTC|imagemeta.CONFIG)
}

func TestGoldenXMPMushroom(t *testing.T) {
	compareWithExiftoolOutput(t, "mushroom.jpg", imagemeta.XMP)
}

func TestGoldenXMP(t *testing.T) {
	// We do verify the "golden" tag count above, but ...
	t.Skip("XMP parsing is currently limited and the diff set is too large to reasoun about.")
	withGolden(t, imagemeta.XMP)
}

func TestGoldenTagCountEXIF(t *testing.T) {
	assertGoldenInfoTagCount(t, "IPTC-PhotometadataRef-Std2021.1.jpg", imagemeta.EXIF)
	assertGoldenInfoTagCount(t, "metadata_demo_exif_only.jpg", imagemeta.EXIF)
	assertGoldenInfoTagCount(t, "bep/sunrise.jpg", imagemeta.EXIF)
}

func TestGoldenTagCountIPTC(t *testing.T) {
	assertGoldenInfoTagCount(t, "metadata_demo_iim_and_xmp_only.jpg", imagemeta.IPTC)
}

func TestGoldenTagCountXMP(t *testing.T) {
	assertGoldenInfoTagCount(t, "bep/sunrise.jpg", imagemeta.XMP)
}

func TestLatLong(t *testing.T) {
	c := qt.New(t)

	_, tags, err := extractTags(t, "bep/sunrise.jpg", imagemeta.EXIF)
	c.Assert(err, qt.IsNil)

	lat, long, err := tags.GetLatLong()
	c.Assert(err, qt.IsNil)
	c.Assert(lat, eq, float64(36.59744166))
	c.Assert(long, eq, float64(-4.50846))

	_, tags, err = extractTags(t, "goexif/geodegrees_as_string.jpg", imagemeta.EXIF)
	c.Assert(err, qt.IsNil)
	lat, long, err = tags.GetLatLong()
	c.Assert(err, qt.IsNil)
	c.Assert(lat, eq, float64(52.013888888))
	c.Assert(long, eq, float64(11.002777))
}

func TestGetDateTime(t *testing.T) {
	c := qt.New(t)

	_, tags, err := extractTags(t, "bep/sunrise.jpg", imagemeta.EXIF)
	c.Assert(err, qt.IsNil)
	d, err := tags.GetDateTime()
	c.Assert(err, qt.IsNil)
	c.Assert(d.Format("2006-01-02"), qt.Equals, "2017-10-27")
}

// TestGetDateTimeFallback tests that GetDateTime falls back to XMP and IPTC
// when EXIF is not available.
func TestGetDateTimeFallback(t *testing.T) {
	c := qt.New(t)

	// This file has no EXIF data, only IPTC and XMP.
	// XMP has: DateCreated: "2017:05:29 17:19:21-04:00", CreateDate: "2017:05:29 17:19:21-04:00"
	// IPTC has: DateCreated: "2017:05:29", TimeCreated: "17:19:21-04:00"
	_, tags, err := extractTags(t, "metadata_demo_iim_and_xmp_only.jpg", imagemeta.IPTC|imagemeta.XMP)
	c.Assert(err, qt.IsNil)

	d, err := tags.GetDateTime()
	c.Assert(err, qt.IsNil)
	c.Assert(d.Format("2006-01-02"), qt.Equals, "2017-05-29")
	// Should preserve the timezone from XMP.
	_, offset := d.Zone()
	c.Assert(offset, qt.Equals, -4*60*60) // -04:00 = -4 hours = -14400 seconds
}

// TestGetDateTimeFallbackIPTC tests that GetDateTime falls back to IPTC
// when both EXIF and XMP are not available.
func TestGetDateTimeFallbackIPTC(t *testing.T) {
	c := qt.New(t)

	// Only load IPTC, so we test the IPTC fallback path.
	_, tags, err := extractTags(t, "metadata_demo_iim_and_xmp_only.jpg", imagemeta.IPTC)
	c.Assert(err, qt.IsNil)

	d, err := tags.GetDateTime()
	c.Assert(err, qt.IsNil)
	c.Assert(d.Format("2006-01-02"), qt.Equals, "2017-05-29")
	// IPTC TimeCreated: "17:19:21-04:00" should preserve timezone.
	_, offset := d.Zone()
	c.Assert(offset, qt.Equals, -4*60*60) // -04:00
}

// TestLatLongFallback tests that GetLatLong falls back to XMP
// when EXIF is not available.
func TestLatLongFallback(t *testing.T) {
	c := qt.New(t)

	// This file has no EXIF GPS data, only XMP GPS data.
	// XMP has GPS coordinates in DMS format: "26,34.951N", "80,12.014W"
	// which get parsed to decimal: 26.5825166..., -80.2002333...
	_, tags, err := extractTags(t, "metadata_demo_iim_and_xmp_only.jpg", imagemeta.XMP)
	c.Assert(err, qt.IsNil)

	lat, long, err := tags.GetLatLong()
	c.Assert(err, qt.IsNil)
	c.Assert(lat, eq, float64(26.5825166666667))
	c.Assert(long, eq, float64(-80.2002333333333))
}

// TestLatLongWithBothSources tests that GetLatLong prefers EXIF over XMP.
func TestLatLongWithBothSources(t *testing.T) {
	c := qt.New(t)

	// sunrise.jpg has EXIF GPS data
	_, tags, err := extractTags(t, "bep/sunrise.jpg", imagemeta.EXIF|imagemeta.XMP)
	c.Assert(err, qt.IsNil)

	lat, long, err := tags.GetLatLong()
	c.Assert(err, qt.IsNil)
	c.Assert(lat, eq, float64(36.59744166))
	c.Assert(long, eq, float64(-4.50846))
}

func TestTagSource(t *testing.T) {
	c := qt.New(t)
	sources := imagemeta.EXIF | imagemeta.IPTC
	c.Assert(sources.Has(imagemeta.EXIF), qt.Equals, true)
	c.Assert(sources.Has(imagemeta.IPTC), qt.Equals, true)
	c.Assert(sources.Has(imagemeta.XMP), qt.Equals, false)
	sources = sources.Remove(imagemeta.EXIF)
	c.Assert(sources.Has(imagemeta.EXIF), qt.Equals, false)
	c.Assert(sources.Has(imagemeta.IPTC), qt.Equals, true)
	c.Assert(sources.IsZero(), qt.Equals, false)
	sources = sources.Remove(imagemeta.IPTC)
	c.Assert(sources.IsZero(), qt.Equals, true)
}

type goldenFileInfo struct {
	ExifTool  map[string]any
	File      map[string]any
	EXIF      map[string]any
	IPTC      map[string]any
	XMP       map[string]any
	Composite map[string]any
	Config    imagemeta.ImageConfig
}

func getSunrise(c *qt.C, imageFormat imagemeta.ImageFormat) (io.ReadSeeker, func()) {
	ext := ""
	switch imageFormat {
	case imagemeta.JPEG:
		ext = ".jpg"
	case imagemeta.WebP:
		ext = ".webp"
	case imagemeta.PNG:
		ext = ".png"
	case imagemeta.TIFF:
		ext = ".tif"
	case imagemeta.AVIF:
		ext = ".avif"
	default:
		c.Fatalf("unknown image format: %v", imageFormat)
	}

	img, err := os.Open(filepath.Join("testdata", "images", "bep/sunrise"+ext))
	c.Assert(err, qt.IsNil)
	return img, func() {
		img.Close()
	}
}

func assertGoldenInfoTagCount(t testing.TB, filename string, sources imagemeta.Source) {
	c := qt.New(t)

	shouldHandle := func(ti imagemeta.TagInfo) bool {
		return true
	}

	_, tags, err := extractTagsWithFilter(t, filename, sources, shouldHandle)
	c.Assert(err, qt.IsNil)
	all := tags.All()

	// Our XMP parsing is currently a little limited so be a little lenient with the assertions.
	hasXMP := sources.Has(imagemeta.XMP)

	c.Assert(len(all) > 0, qt.IsTrue)

	goldenInfo := readGoldenInfo(t, filename)
	tagsLeft := make(map[string]imagemeta.TagInfo)
	tagsRight := make(map[string]any)

	if sources.Has(imagemeta.EXIF) {
		maps.Copy(tagsLeft, tags.EXIF())
		maps.Copy(tagsRight, goldenInfo.EXIF)
	}
	if sources.Has(imagemeta.IPTC) {
		maps.Copy(tagsLeft, tags.IPTC())
		maps.Copy(tagsRight, goldenInfo.IPTC)
	}
	if sources.Has(imagemeta.XMP) {
		maps.Copy(tagsLeft, tags.XMP())
		maps.Copy(tagsRight, goldenInfo.XMP)
	}

	count := 0

	var keysLeft []string
	for k := range tagsLeft {
		keysLeft = append(keysLeft, k)
	}

	var keysRight []string
	for k := range tagsRight {
		keysRight = append(keysRight, k)
	}

	sort.Strings(keysRight)
	sort.Strings(keysLeft)

	if !hasXMP {

		for _, k := range keysRight {
			if _, found := tagsLeft[k]; !found {
				t.Log("Missing tag: ", k, "=>", tagsRight[k])
				count++
			}
			if count > 10 {
				break
			}
		}

		count = 0

		for _, k := range keysLeft {
			if _, found := tagsRight[k]; !found {
				t.Log("Extra tag: ", k, "=>", tagsLeft[k].Value)
				count++
			}
			if count > 10 {
				break
			}
		}
	}

	if hasXMP {
		diff := len(tagsRight) - len(tagsLeft)
		c.Assert(diff < 50, qt.IsTrue)
	} else {
		c.Assert(len(tagsLeft), qt.Equals, len(tagsRight))
	}
}

func compareWithExiftoolOutput(t testing.TB, filename string, sources imagemeta.Source) {
	c := qt.New(t)
	res, tags, err := extractTags(t, filename, sources)
	c.Assert(err, qt.IsNil)
	all := tags.All()
	goldenInfo := readGoldenInfo(t, filename)

	if sources.Has(imagemeta.CONFIG) {
		c.Assert(res.ImageConfig, qt.DeepEquals, goldenInfo.Config, qt.Commentf("config mismatch for file %q", filename))
	}

	var tagsSorted []imagemeta.TagInfo
	for _, v := range all {
		c.Assert(sources.Has(v.Source), qt.IsTrue, qt.Commentf("source: %s should not be in list for file %q", v.Source, filename))
		tagsSorted = append(tagsSorted, v)
	}
	sort.Slice(tagsSorted, func(i, j int) bool {
		return tagsSorted[i].Tag < tagsSorted[j].Tag
	})

	xmpReplacer := strings.NewReplacer(
		"true", "True",
	)

	for _, v := range tagsSorted {
		normalizeUs := func(s string, our any, source imagemeta.Source) any {
			// JSON umarshaled to a map has very limited types.
			// Normalize to make them comparable.
			switch v := our.(type) {
			case imagemeta.Rat[uint32]:
				return v.Float64()
			case imagemeta.Rat[int32]:
				return v.Float64()
			case float64:
				return v
			case int64:
				return float64(v)
			case uint32:
				return float64(v)
			case int32:
				return float64(v)
			case uint16:
				return float64(v)
			case uint8:
				return float64(v)
			case int:
				return float64(v)
			default:
				return v
			}
		}

		normalizeThem := func(s string, v any, source imagemeta.Source) any {
			if source == imagemeta.XMP {
				// Normalize exiftool date format to ISO format.
				normalizeXMPDate := func(s string) string {
					if len(s) >= 19 && s[4] == ':' && s[7] == ':' && s[10] == ' ' {
						// Convert "2024:09:29 10:37:52" to "2024-09-29T10:37:52"
						return s[:4] + "-" + s[5:7] + "-" + s[8:10] + "T" + s[11:]
					}
					return s
				}
				switch v := v.(type) {
				case []string:
					if len(v) == 1 {
						return normalizeXMPDate(v[0])
					}
					return v
				case []any:
					if len(v) == 1 {
						return normalizeXMPDate(fmt.Sprintf("%v", v[0]))
					}
					// Convert to a string slice.
					vvv := make([]string, len(v))
					for i, vv := range v {
						vvv[i] = fmt.Sprintf("%v", vv)
					}
					return vvv
				default:
					return normalizeXMPDate(xmpReplacer.Replace(fmt.Sprintf("%v", v)))
				}
			}

			switch v := v.(type) {
			case string:
				v = strings.TrimSpace(v)
				if strings.Contains(v, "Binary data") {
					return strings.Replace(v, ", use -b option to extract", "", 1)
				}
				switch s {
				case "ShutterSpeedValue", "SubSecTimeDigitized", "SubSecTimeOriginal", "GPSSatellites":
					f, _ := strconv.ParseFloat(v, 64)
					return f
				case "WhiteBalance":
					if strings.TrimSpace(v) == "AUTO1" {
						return float64(0)
					}
				case "CodedCharacterSet":
					if v == "\x1b%G" || v == "UTF8" {
						return "UTF-8"
					}
					return "ISO-8859-1"

				}
				return v
			case float64:
				switch s {
				case "SerialNumber", "LensSerialNumber", "ObjectName":
					return fmt.Sprintf("%d", int(v))
				case "Software":
					// ExifTool may output numeric-looking software versions (e.g. iOS "26.3")
					// as float64 in JSON. Convert back to string for comparison.
					return strconv.FormatFloat(v, 'f', -1, 64)
				}
				if source == imagemeta.IPTC {
					switch s {
					case "ApplicationRecordVersion", "EnvelopeRecordVersion", "FileFormat", "FileVersion", "MaxSubfileSize", "ObjectSizeAnnounced", "SizeMode":
						return v
					default:
						return fmt.Sprintf("%v", v)
					}
				}
			case []any:
				vvv := make([]string, len(v))
				for i, vv := range v {
					vvv[i] = fmt.Sprintf("%v", vv)
				}
				return vvv
			}

			return v
		}

		var exifToolValue any
		var found bool

		switch v.Source {
		case imagemeta.EXIF:
			exifToolValue, found = goldenInfo.EXIF[v.Tag]
		case imagemeta.IPTC:
			exifToolValue, found = goldenInfo.IPTC[v.Tag]
		case imagemeta.XMP:
			exifToolValue, found = goldenInfo.XMP[v.Tag]
		}

		if found {
			// Skip comparison for binary data tags (exiftool shows
			// "(Binary data N bytes, use -b option to extract)" for large arrays).
			if s, ok := exifToolValue.(string); ok && strings.Contains(s, "Binary data") {
				continue
			}
			expect := normalizeThem(v.Tag, exifToolValue, v.Source)
			got := normalizeUs(v.Tag, v.Value, v.Source)
			if v, ok := got.(float64); ok {
				if math.IsInf(v, 1) {
					panic(fmt.Errorf("inf: %v", v))
				}
				if math.IsInf(v, -1) {
					panic(fmt.Errorf("-inf: %v", v))
				}
				if math.IsNaN(v) {
					panic(fmt.Errorf("nan: %v", v))
				}
			}
			c.Assert(got, eq, expect, qt.Commentf("%s (%s): got: %T/%T %v %q\n\n%s\n\n%s", v.Tag, v.Source, got, expect, v.Value, filename, got, expect))
		}
	}
}

func extToFormat(ext string) imagemeta.ImageFormat {
	switch ext {
	case ".jpg":
		return imagemeta.JPEG
	case ".webp":
		return imagemeta.WebP
	case ".png":
		return imagemeta.PNG
	case ".tif", ".tiff":
		return imagemeta.TIFF
	case ".heic", ".heif":
		return imagemeta.HEIF
	case ".avif":
		return imagemeta.AVIF
	case ".dng":
		return imagemeta.DNG
	case ".cr2":
		return imagemeta.CR2
	case ".nef":
		return imagemeta.NEF
	case ".arw":
		return imagemeta.ARW
	case ".pef":
		return imagemeta.PEF
	case ".txt":
		return -1
	default:
		panic(fmt.Errorf("unknown image format: %s", ext))
	}
}

func extractTags(t testing.TB, filename string, sources imagemeta.Source, opts ...withOptions) (imagemeta.DecodeResult, imagemeta.Tags, error) {
	shouldHandle := func(ti imagemeta.TagInfo) bool {
		// Drop the thumbnail tags.
		return ti.Namespace != "IFD1"
	}
	return extractTagsWithFilter(t, filename, sources, shouldHandle, opts...)
}

type withOptions func(opts *imagemeta.Options)

func extractTagsWithFilter(t testing.TB, filename string, sources imagemeta.Source, shouldHandle func(ti imagemeta.TagInfo) bool, opts ...withOptions) (imagemeta.DecodeResult, imagemeta.Tags, error) {
	t.Helper()
	if !filepath.IsAbs(filename) {
		filename = filepath.Join("testdata", "images", filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var tags imagemeta.Tags
	handleTag := func(ti imagemeta.TagInfo) error {
		tags.Add(ti)
		return nil
	}

	imageFormat := extToFormat(filepath.Ext(filename))
	if imageFormat == -1 {
		return imagemeta.DecodeResult{}, tags, nil
	}

	knownWarnings := []*regexp.Regexp{}

	warnf := func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		for _, re := range knownWarnings {
			if re.MatchString(s) {
				return
			}
		}
		panic(errors.New(s))
	}

	imgOpts := imagemeta.Options{R: f, ImageFormat: imageFormat, ShouldHandleTag: shouldHandle, HandleTag: handleTag, Warnf: warnf, Sources: sources}
	for _, opt := range opts {
		opt(&imgOpts)
	}

	res, err := imagemeta.Decode(imgOpts)
	if err != nil {
		return res, tags, err
	}

	// See https://github.com/gohugoio/hugo/issues/12741 and https://github.com/golang/go/issues/59627
	// Verify that it can be marshaled to JSON.
	_, err = json.Marshal(tags.All())
	if err != nil {
		for tag, ti := range tags.All() {
			_, err2 := json.Marshal(ti.Value)
			if err2 != nil {
				t.Fatal(fmt.Errorf("failed to marshal tag %q in source %s with value %v/%T to JSON: %w", tag, ti.Source, ti.Value, ti.Value, err2))
			}
		}
		t.Fatal(fmt.Errorf("failed to marshal tags in %q to JSON: %w", filename, err))
	}

	return res, tags, nil
}

func readGoldenInfo(t testing.TB, filename string) goldenFileInfo {
	exiftoolsJSONFilename := filepath.Join("gen", "testdata_exiftool", "images", filename+".json")
	var exifToolValue []goldenFileInfo
	b, err := os.ReadFile(exiftoolsJSONFilename)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &exifToolValue); err != nil {
		t.Fatal(err)
	}
	m := exifToolValue[0]
	configJSONFilename := filepath.Join("gen", "testdata_exiftool", "images", filename+".config.json")
	b, err = os.ReadFile(configJSONFilename)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err == nil {
		if err := json.Unmarshal(b, &m.Config); err != nil {
			t.Fatal(err)
		}
	}

	// Normalise the IPTC keys and tags.
	for k, v := range m.IPTC {
		if strings.Contains(k, "-") {
			delete(m.IPTC, k)
			// Exiftool has some weird hypenated keys, e.g. "By-line".
			m.IPTC[strings.ReplaceAll(k, "-", "")] = v
		}
	}
	return m
}

func withGolden(t testing.TB, sources imagemeta.Source) {
	withTestDataFile(t, func(path string, info os.FileInfo, err error) error {
		if strings.HasPrefix(path, "corrupt") {
			return nil
		}

		if goldenSkip[filepath.ToSlash(path)] {
			return nil
		}
		compareWithExiftoolOutput(t, path, sources)
		return nil
	})
}

func withTestDataFile(t testing.TB, fn func(path string, info os.FileInfo, err error) error) {
	t.Helper()
	err := filepath.Walk(filepath.Join("testdata", "images"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || strings.HasPrefix(info.Name(), ".") || strings.HasSuffix(info.Name(), ".txt") {
			return nil
		}

		path = strings.TrimPrefix(path, filepath.Join("testdata", "images")+string(filepath.Separator))

		return fn(path, info, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
}

var goldenSkip = map[string]bool{
	"goexif/geodegrees_as_string.jpg":  true, // The file has many EXIF errors. I think we do a better job than exiftools, but there are some differences.
	"invalid-encoding-usercomment.jpg": true, // The file has an EXIF error that produces a warning in imagemeta. It's tested separately.
	"sample.cr2":                       true, // CR2 chains 4 IFDs; exiftool reports IFD3 StripByteCounts which we don't reach.
	"sample.arw":                       true, // IFD0 0x0201/0x0202 are preview offsets; exiftool renames to PreviewImageStart/Length so values differ.
	"bep/jølstravatnet.pef":            true, // GPSProcessingMethod includes encoding prefix; CONFIG dimensions differ (identify uses libraw active area).
}

var isSpaceDelimitedFloatRe = regexp.MustCompile(`^(\d+\.\d+)( \d+\.?\d*)+$`)

var cmpFloats = func(x, y float64) bool {
	if x == y {
		return true
	}
	delta := math.Abs(x - y)
	mean := math.Abs(x+y) / 2.0
	return delta/mean < 0.00001
}

var eq = qt.CmpEquals(
	cmp.Comparer(func(x, y imagemeta.Rat[uint32]) bool {
		return x.String() == y.String()
	}),

	cmp.Comparer(func(x, y imagemeta.Rat[int32]) bool {
		return x.String() == y.String()
	}),

	cmp.Comparer(func(x, y float64) bool {
		return cmpFloats(x, y)
	}),

	cmp.Comparer(func(x, y string) bool {
		if x == y {
			return true
		}
		if isSpaceDelimitedFloatRe.MatchString(x) && isSpaceDelimitedFloatRe.MatchString(y) {
			floatStringLeft := strings.Fields(x)
			floatStringRight := strings.Fields(y)
			if len(floatStringLeft) != len(floatStringRight) {
				return false
			}
			for i := range floatStringLeft {
				left, err := strconv.ParseFloat(floatStringLeft[i], 64)
				if err != nil {
					return false
				}
				right, err := strconv.ParseFloat(floatStringRight[i], 64)
				if err != nil {
					return false
				}
				if !cmpFloats(left, right) {
					return false
				}
			}
			return true
		}
		return false
	}),
)

func BenchmarkDecode(b *testing.B) {
	handleTag := func(ti imagemeta.TagInfo) error {
		return nil
	}

	sourceSetEXIF := imagemeta.EXIF
	sourceSetIPTC := imagemeta.IPTC
	sourceSetAll := imagemeta.EXIF | imagemeta.IPTC | imagemeta.XMP

	runBenchmark := func(b *testing.B, name string, imageFormat imagemeta.ImageFormat, f func(r io.ReadSeeker) error) {
		img, close := getSunrise(qt.New(b), imageFormat)
		b.Cleanup(close)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if err := f(img); err != nil {
					b.Fatal(err)
				}
				img.Seek(0, 0)
			}
		})
	}

	imageFormat := imagemeta.PNG
	runBenchmark(b, "exif/png", imagemeta.PNG, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetEXIF})
		return err
	})

	runBenchmark(b, "all/png", imagemeta.PNG, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetAll})
		return err
	})

	runBenchmark(b, "config/png", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: imagemeta.CONFIG})
		return err
	})

	imageFormat = imagemeta.WebP
	runBenchmark(b, "all/webp", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetAll})
		return err
	})

	runBenchmark(b, "xmp/webp", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: imagemeta.XMP})
		return err
	})

	runBenchmark(b, "exif/webp", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetEXIF})
		return err
	})

	runBenchmark(b, "config/webp", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: imagemeta.CONFIG})
		return err
	})

	imageFormat = imagemeta.JPEG
	runBenchmark(b, "exif/jpg", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetEXIF})
		return err
	})

	runBenchmark(b, "iptc/jpg", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetIPTC})
		return err
	})

	runBenchmark(b, "iptc/jpg/category", imageFormat, func(r io.ReadSeeker) error {
		shouldHandle := func(ti imagemeta.TagInfo) bool {
			return ti.Tag == "Category"
		}
		handleTag := func(ti imagemeta.TagInfo) error {
			if ti.Tag == "Category" {
				return imagemeta.ErrStopWalking
			}
			return nil
		}
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, ShouldHandleTag: shouldHandle, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetIPTC})
		return err
	})

	runBenchmark(b, "iptc/jpg/city", imageFormat, func(r io.ReadSeeker) error {
		shouldHandle := func(ti imagemeta.TagInfo) bool {
			return ti.Tag == "City"
		}
		handleTag := func(ti imagemeta.TagInfo) error {
			if ti.Tag == "City" {
				return imagemeta.ErrStopWalking
			}
			return nil
		}
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, ShouldHandleTag: shouldHandle, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetIPTC})
		return err
	})

	runBenchmark(b, "xmp/jpg", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: imagemeta.XMP})
		return err
	})

	runBenchmark(b, "all/jpg", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetAll})
		return err
	})

	runBenchmark(b, "config/jpg", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: imagemeta.CONFIG})
		return err
	})

	imageFormat = imagemeta.TIFF
	runBenchmark(b, "exif/tiff", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetEXIF})
		return err
	})
	runBenchmark(b, "iptc/tiff", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetIPTC})
		return err
	})
	runBenchmark(b, "all/tiff", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetAll})
		return err
	})
	runBenchmark(b, "config/tiff", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: imagemeta.CONFIG})
		return err
	})

	imageFormat = imagemeta.AVIF
	runBenchmark(b, "all/avif", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetAll})
		return err
	})
	runBenchmark(b, "config/avif", imageFormat, func(r io.ReadSeeker) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: imagemeta.CONFIG})
		return err
	})

	runBenchmarkWithFile := func(b *testing.B, name string, imageFormat imagemeta.ImageFormat, filename string, f func(r io.ReadSeeker) error) {
		img, err := os.Open(filepath.Join("testdata", "images", filename))
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { img.Close() })
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if err := f(img); err != nil {
					b.Fatal(err)
				}
				img.Seek(0, 0)
			}
		})
	}

	for _, tt := range []struct {
		name     string
		format   imagemeta.ImageFormat
		filename string
	}{
		{"dng", imagemeta.DNG, "sample.dng"},
		{"cr2", imagemeta.CR2, "sample.cr2"},
		{"nef", imagemeta.NEF, "sample.nef"},
		{"arw", imagemeta.ARW, "sample.arw"},
		{"pef", imagemeta.PEF, "bep/jølstravatnet.pef"},
	} {
		imageFormat = tt.format
		runBenchmarkWithFile(b, "exif/"+tt.name, tt.format, tt.filename, func(r io.ReadSeeker) error {
			_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetEXIF})
			return err
		})
		runBenchmarkWithFile(b, "all/"+tt.name, tt.format, tt.filename, func(r io.ReadSeeker) error {
			_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: sourceSetAll})
			return err
		})
		runBenchmarkWithFile(b, "config/"+tt.name, tt.format, tt.filename, func(r io.ReadSeeker) error {
			_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Warnf: panicWarnf, Sources: imagemeta.CONFIG})
			return err
		})
	}
}

func BenchmarkDecodeCompareWithGoexif(b *testing.B) {
	runBenchmark := func(b *testing.B, name string, imageFormat imagemeta.ImageFormat, f func(r io.ReadSeeker) error) {
		img, close := getSunrise(qt.New(b), imageFormat)
		b.Cleanup(close)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if err := f(img); err != nil {
					b.Fatal(err)
				}
				img.Seek(0, 0)
			}
		})
	}

	imageFormat := imagemeta.JPEG
	for _, imageFormat := range []imagemeta.ImageFormat{imagemeta.JPEG} {
		name := strings.ToLower(imageFormat.String())
		runBenchmark(b, fmt.Sprintf("bep/imagemeta/exif/%s/alltags", name), imageFormat, func(r io.ReadSeeker) error {
			_, err := imagemeta.Decode(imagemeta.Options{
				R: r, ImageFormat: imageFormat,

				HandleTag: func(ti imagemeta.TagInfo) error {
					return nil
				},
				Sources: imagemeta.EXIF,
			})
			return err
		})

		runBenchmark(b, fmt.Sprintf("bep/imagemeta/exif/%s/orientation", name), imageFormat, func(r io.ReadSeeker) error {
			_, err := imagemeta.Decode(imagemeta.Options{
				R: r, ImageFormat: imageFormat,
				ShouldHandleTag: func(ti imagemeta.TagInfo) bool {
					return ti.Tag == "Orientation"
				},
				HandleTag: func(ti imagemeta.TagInfo) error {
					if ti.Tag == "Orientation" {
						return imagemeta.ErrStopWalking
					}
					return nil
				},
				Sources: imagemeta.EXIF,
			})
			return err
		})
	}

	runBenchmark(b, "rwcarlsen/goexif/exif/jpg/alltags", imageFormat, func(r io.ReadSeeker) error {
		_, err := exif.Decode(r)
		return err
	})
}

func panicWarnf(format string, args ...any) {
	panic(fmt.Errorf(format, args...))
}
