package imagemeta_test

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/bep/imagemeta"
	"github.com/rwcarlsen/goexif/exif"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
)

func TestDecodeBasic(t *testing.T) {
	c := qt.New(t)

	for _, imageFormat := range []imagemeta.ImageFormat{imagemeta.ImageFormatJPEG, imagemeta.ImageFormatPNG, imagemeta.ImageFormatWebP} {
		c.Run(fmt.Sprintf("%v", imageFormat), func(c *qt.C) {
			img, close := getSunrise(c, imageFormat)
			c.Cleanup(close)

			tags := make(map[string]imagemeta.TagInfo)
			handleTag := func(ti imagemeta.TagInfo) error {
				tags[ti.Tag] = ti
				return nil
			}

			err := imagemeta.Decode(imagemeta.Options{R: img, ImageFormat: imageFormat, HandleTag: handleTag})
			c.Assert(err, qt.IsNil)

			c.Assert(tags["Orientation"].Value, qt.Equals, uint16(1))
			c.Assert(tags["ExposureTime"].Value, eq, big.NewRat(1, 200))
			if imageFormat != imagemeta.ImageFormatPNG { // No IPTC in PNG.
				c.Assert(tags["Headline"].Value, qt.Equals, "Sunrise in Spain")
				c.Assert(tags["Copyright"].Value, qt.Equals, "Bjørn Erik Pedersen")
			}

			// TODO1 InteroperabilityIndex
		})
	}
}

func TestDecodeIPTCReference(t *testing.T) {
	c := qt.New(t)
	const filename = "IPTC-PhotometadataRef-Std2021.1.jpg"

	img, err := os.Open(filepath.Join("testdata", filename))
	c.Assert(err, qt.IsNil)

	c.Cleanup(func() {
		c.Assert(img.Close(), qt.IsNil)
	})

	tags := make(map[string]imagemeta.TagInfo)
	handleTag := func(ti imagemeta.TagInfo) error {
		if _, seen := tags[ti.Tag]; seen {
			c.Fatalf("duplicate tag: %s", ti.Tag)
		}
		c.Assert(ti.Tag, qt.Not(qt.Contains), "Unknown")
		tags[ti.Tag] = ti
		return nil
	}

	err = imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.ImageFormatJPEG,
			HandleTag:   handleTag,
			Sources:     imagemeta.TagSourceIPTC,
		},
	)
	c.Assert(err, qt.IsNil)

	c.Assert(len(tags), qt.Equals, 22)
	c.Assert(tags["Byline"].Value, qt.Equals, "Creator1 (ref2021.1)")
	c.Assert(tags["BylineTitle"].Value, qt.Equals, "Creator's Job Title  (ref2021.1)")
	c.Assert(tags["RecordVersion"].Value, qt.Equals, uint16(4))
	c.Assert(tags["DateCreated"].Value, qt.Equals, "20211020")
	c.Assert(tags["Keywords"].Value, qt.DeepEquals, []string{"Keyword1ref2021.1", "Keyword2ref2021.1", "Keyword3ref2021.1"})

}

func TestDecodeOrientationOnly(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c, imagemeta.ImageFormatJPEG)
	c.Cleanup(close)

	tags := make(map[string]imagemeta.TagInfo)
	handleTag := func(ti imagemeta.TagInfo) error {
		if ti.Tag == "Orientation" {
			tags[ti.Tag] = ti
			return imagemeta.ErrStopWalking
		}
		return nil
	}

	err := imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.ImageFormatJPEG,
			HandleTag:   handleTag,
			Sources:     imagemeta.TagSourceEXIF,
		},
	)

	c.Assert(err, qt.IsNil)
	c.Assert(tags["Orientation"].Value, qt.Equals, uint16(1))
	c.Assert(len(tags), qt.Equals, 1)

}

func TestSmoke(t *testing.T) {
	c := qt.New(t)

	// Test the images in the testdata/smoke folder and make sure we get a sensible result for each.
	// The primary goal of this test is to make sure we don't crash on any of them.

	files, err := filepath.Glob(filepath.Join("testdata", "smoke", "*.*"))
	c.Assert(err, qt.IsNil)

	for _, file := range files {
		img, err := os.Open(file)
		c.Assert(err, qt.IsNil)
		format := extToFormat(filepath.Ext(file))
		tags := make(map[string]imagemeta.TagInfo)
		handleTag := func(ti imagemeta.TagInfo) error {
			tags[ti.Tag] = ti
			return nil
		}
		err = imagemeta.Decode(imagemeta.Options{R: img, ImageFormat: format, HandleTag: handleTag})
		c.Assert(err, qt.IsNil)
		c.Assert(len(tags), qt.Not(qt.Equals), 0)
		img.Close()
	}

}

func TestCorrupt(t *testing.T) {
	c := qt.New(t)

	files, err := filepath.Glob(filepath.Join("testdata", "corrupt", "*.*"))
	c.Assert(err, qt.IsNil)

	for _, file := range files {
		img, err := os.Open(file)
		c.Assert(err, qt.IsNil)
		format := extToFormat(filepath.Ext(file))
		handleTag := func(ti imagemeta.TagInfo) error {
			return nil
		}
		err = imagemeta.Decode(imagemeta.Options{R: img, ImageFormat: format, HandleTag: handleTag})
		c.Assert(err, qt.Equals, imagemeta.ErrInvalidFormat)
		img.Close()
	}

}

func extToFormat(ext string) imagemeta.ImageFormat {
	switch ext {
	case ".jpg":
		return imagemeta.ImageFormatJPEG
	case ".webp":
		return imagemeta.ImageFormatWebP
	case ".png":
		return imagemeta.ImageFormatPNG
	default:
		panic("unknown image format")
	}
}

func getSunrise(c *qt.C, imageFormat imagemeta.ImageFormat) (imagemeta.Reader, func()) {
	ext := ""
	switch imageFormat {
	case imagemeta.ImageFormatJPEG:
		ext = ".jpg"
	case imagemeta.ImageFormatWebP:
		ext = ".webp"
	case imagemeta.ImageFormatPNG:
		ext = ".png"
	default:
		c.Fatalf("unknown image format: %v", imageFormat)
	}

	img, err := os.Open(filepath.Join("testdata", "sunrise"+ext))
	c.Assert(err, qt.IsNil)
	return img, func() {
		img.Close()
	}
}

var eq = qt.CmpEquals(
	cmp.Comparer(func(x, y *big.Rat) bool {
		return x.RatString() == y.RatString()
	}),

	cmp.Comparer(func(x, y float64) bool {
		delta := math.Abs(x - y)
		mean := math.Abs(x+y) / 2.0
		return delta/mean < 0.00001
	}),
)

func BenchmarkDecode(b *testing.B) {

	handleTag := func(ti imagemeta.TagInfo) error {
		return nil
	}

	sourceSetEXIF := imagemeta.TagSourceEXIF
	sourceSetIPTC := imagemeta.TagSourceIPTC
	sourceSetAll := imagemeta.TagSourceEXIF | imagemeta.TagSourceIPTC | imagemeta.TagSourceXMP

	runBenchmark := func(b *testing.B, name string, imageFormat imagemeta.ImageFormat, f func(r imagemeta.Reader) error) {
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

	imageFormat := imagemeta.ImageFormatPNG
	runBenchmark(b, "bep/imagemeta/png/exif", imagemeta.ImageFormatPNG, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetAll})
		return err
	})

	imageFormat = imagemeta.ImageFormatWebP
	runBenchmark(b, "bep/imagemeta/webp/all", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetAll})
		return err
	})

	runBenchmark(b, "bep/imagemeta/webp/exif", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetEXIF})
		return err
	})

	imageFormat = imagemeta.ImageFormatJPEG
	runBenchmark(b, "bep/imagemeta/jpg/exif", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetEXIF})
		return err
	})

	runBenchmark(b, "bep/imagemeta/jpg/iptc", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetIPTC})
		return err
	})

	runBenchmark(b, "bep/imagemeta/jpg/all", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetAll})
		return err
	})

	runBenchmark(b, "rwcarlsen/goexif", imageFormat, func(r imagemeta.Reader) error {
		_, err := exif.Decode(r)
		return err
	})

}
