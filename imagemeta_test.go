package goexif_test

import (
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bep/goexif"
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

	meta, err := goexif.Decode(goexif.Options{R: img})
	c.Assert(err, qt.IsNil)
	c.Assert(meta, qt.IsNotNil)
	//fmt.Printf("%+v\n", tags)
	c.Assert(meta.Orientation(), qt.Equals, goexif.OrientationNormal)
	c.Assert(meta.DateTime(time.UTC).IsZero(), qt.IsFalse)
	c.Assert(meta.EXIF["ExposureTime"], eq, big.NewRat(1, 200))
	c.Assert(meta.IPTC["Headline"], qt.Equals, "Sunrise in Spain")
	c.Assert(meta.IPTC["Copyright"], qt.Equals, "Bjørn Erik Pedersen")

}

func TestDecodeLatLong(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c)
	c.Cleanup(close)

	tags, err := goexif.Decode(goexif.Options{R: img})
	c.Assert(err, qt.IsNil)

	lat, long := tags.LatLong()
	c.Assert(lat, eq, 36.59744)
	c.Assert(long, eq, -4.50846)
}

func TestDecodeOrientationOnly(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c)
	c.Cleanup(close)

	meta, err := goexif.Decode(
		goexif.Options{
			R: img,
			TagSet: map[string]bool{
				"Orientation": true,
			},
		},
	)

	c.Assert(err, qt.IsNil)
	c.Assert(meta.Orientation(), qt.Equals, goexif.OrientationNormal)
	c.Assert(len(meta.EXIF), qt.Equals, 1)
}

func getSunrise(c *qt.C) (goexif.Reader, func()) {
	img, err := os.Open(filepath.Join("testdata", "sunrise.jpg"))
	c.Assert(err, qt.IsNil)
	return img, func() {
		img.Close()
	}
}
