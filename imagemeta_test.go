package imagemeta_test

import (
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rwcarlsen/goexif/exif"

	"github.com/bep/imagemeta"
	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
)

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

func TestDecodeBasic(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c)
	c.Cleanup(close)

	meta, err := imagemeta.Decode(imagemeta.Options{R: img})
	c.Assert(err, qt.IsNil)
	c.Assert(meta, qt.IsNotNil)
	//fmt.Printf("%+v\n", tags)
	c.Assert(meta.Orientation(), qt.Equals, imagemeta.OrientationNormal)
	c.Assert(meta.DateTime(time.UTC).IsZero(), qt.IsFalse)
	c.Assert(meta.EXIF["ExposureTime"], eq, big.NewRat(1, 200))
	c.Assert(meta.IPTC["Headline"], qt.Equals, "Sunrise in Spain")
	c.Assert(meta.IPTC["Copyright"], qt.Equals, "Bjørn Erik Pedersen")

}

func TestDecodeLatLong(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c)
	c.Cleanup(close)

	tags, err := imagemeta.Decode(imagemeta.Options{R: img})
	c.Assert(err, qt.IsNil)

	lat, long := tags.LatLong()
	c.Assert(lat, eq, 36.59744)
	c.Assert(long, eq, -4.50846)
}

func TestDecodeOrientationOnly(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c)
	c.Cleanup(close)

	meta, err := imagemeta.Decode(
		imagemeta.Options{
			R: img,
			TagSet: map[string]bool{
				"Orientation": true,
			},
		},
	)

	c.Assert(err, qt.IsNil)
	c.Assert(meta.Orientation(), qt.Equals, imagemeta.OrientationNormal)
	c.Assert(len(meta.EXIF), qt.Equals, 1)
	c.Assert(len(meta.IPTC), qt.Equals, 0)
}

func getSunrise(c *qt.C) (imagemeta.Reader, func()) {
	img, err := os.Open(filepath.Join("testdata", "sunrise.jpg"))
	c.Assert(err, qt.IsNil)
	return img, func() {
		img.Close()
	}
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
		meta, err := imagemeta.Decode(imagemeta.Options{R: img})
		c.Assert(err, qt.IsNil)
		c.Assert(meta, qt.Not(qt.IsNil))
		c.Assert(len(meta.EXIF)+len(meta.IPTC), qt.Not(qt.Equals), 0)
		img.Close()
	}

}

func BenchmarkDecode(b *testing.B) {

	runBenchmark := func(b *testing.B, name string, f func(r imagemeta.Reader) error) {
		img, close := getSunrise(qt.New(b))
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

	runBenchmark(b, "bep/imagemeta", func(r imagemeta.Reader) error {
		_, err := imagemeta.Decode(imagemeta.Options{R: r, SkipITPC: true})
		return err
	})

	runBenchmark(b, "rwcarlsen/goexif", func(r imagemeta.Reader) error {
		_, err := exif.Decode(r)
		return err
	})

}
