package imagemeta

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/rwcarlsen/goexif/exif"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
)

func TestDecodeBasic(t *testing.T) {
	c := qt.New(t)

	// ImageFormatJPEG,

	for _, imageFormat := range []ImageFormat{ImageFormatJPEG, ImageFormatWebP} {
		c.Run(fmt.Sprintf("%v", imageFormat), func(c *qt.C) {
			img, close := getSunrise(c, imageFormat)
			c.Cleanup(close)

			tags := make(map[string]TagInfo)
			handleTag := func(ti TagInfo) error {
				tags[ti.Tag] = ti
				return nil
			}

			err := Decode(Options{R: img, ImageFormat: imageFormat, HandleTag: handleTag})
			c.Assert(err, qt.IsNil)

			c.Assert(tags["Orientation"].Value, qt.Equals, uint16(1))
			c.Assert(tags["ExposureTime"].Value, eq, big.NewRat(1, 200))
			c.Assert(tags["Headline"].Value, qt.Equals, "Sunrise in Spain")
			c.Assert(tags["Copyright"].Value, qt.Equals, "Bjørn Erik Pedersen")

			// TODO1 InteroperabilityIndex
		})
	}
}

func TestDecodeOrientationOnly(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c, ImageFormatJPEG)
	c.Cleanup(close)

	tags := make(map[string]TagInfo)
	handleTag := func(ti TagInfo) error {
		if ti.Tag == "Orientation" {
			tags[ti.Tag] = ti
			return ErrStopWalking
		}
		return nil
	}

	err := Decode(
		Options{
			R:           img,
			ImageFormat: ImageFormatJPEG,
			HandleTag:   handleTag,
			SourceSet:   map[TagSource]bool{TagSourceEXIF: true},
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
		format := ImageFormatJPEG
		if filepath.Ext(file) == ".webp" {
			format = ImageFormatWebP
		}

		c.Assert(err, qt.IsNil)
		tags := make(map[string]TagInfo)
		handleTag := func(ti TagInfo) error {
			tags[ti.Tag] = ti
			return nil
		}
		err = Decode(Options{R: img, ImageFormat: format, HandleTag: handleTag})
		c.Assert(err, qt.IsNil)
		c.Assert(len(tags), qt.Not(qt.Equals), 0)
		img.Close()
	}

}

func getSunrise(c *qt.C, imageFormat ImageFormat) (Reader, func()) {
	ext := ""
	switch imageFormat {
	case ImageFormatJPEG:
		ext = ".jpg"
	case ImageFormatWebP:
		ext = ".webp"
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

	handleTag := func(ti TagInfo) error {
		return nil
	}

	sourceSet := map[TagSource]bool{TagSourceEXIF: true}

	runBenchmark := func(b *testing.B, name string, f func(r Reader) error) {
		img, close := getSunrise(qt.New(b), ImageFormatJPEG)
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

	runBenchmark(b, "bep/imagemeta", func(r Reader) error {
		err := Decode(Options{R: r, ImageFormat: ImageFormatJPEG, HandleTag: handleTag, SourceSet: sourceSet})
		return err
	})

	runBenchmark(b, "rwcarlsen/goexif", func(r Reader) error {
		_, err := exif.Decode(r)
		return err
	})

}
