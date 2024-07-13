package imagemeta_test

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

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

			err := imagemeta.Decode(imagemeta.Options{R: img, ImageFormat: imageFormat, HandleTag: handleTag})
			c.Assert(err, qt.IsNil)

			allTags := tags.All()
			exifTags := tags.EXIF()

			c.Assert(len(allTags), qt.Not(qt.Equals), 0)

			if imageFormat != imagemeta.PNG && imageFormat != imagemeta.TIFF {
				c.Assert(allTags["Headline"].Value, qt.Equals, "Sunrise in Spain")
				c.Assert(allTags["Copyright"].Value, qt.Equals, "Bjørn Erik Pedersen")
			}
			if imageFormat != imagemeta.TIFF { // TODO1
				c.Assert(exifTags["Orientation"].Value, qt.Equals, uint16(1))
				c.Assert(exifTags["ExposureTime"].Value, eq, imagemeta.NewRat[uint32](1, 200))
				c.Assert(exifTags["FocalLength"].Value, eq, imagemeta.NewRat[uint32](21, 1))

			}
		})
	}
}

func TestDecodeWebP(t *testing.T) {
	c := qt.New(t)
	tags := extractTags(t, "sunrise.webp", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP)

	c.Assert(tags.EXIF()["Copyright"].Value, qt.Equals, "Bjørn Erik Pedersen")
	c.Assert(tags.EXIF()["ApertureValue"].Value, eq, 5.6)
	c.Assert(tags.XMP()["CreatorTool"].Value, qt.Equals, "Adobe Photoshop Lightroom Classic 12.4 (Macintosh)")
	// No IPTC in this file, Exiftool stores the IPTC fields in XMP.
	c.Assert(tags.XMP()["City"].Value, qt.Equals, "Benalmádena")
}

func TestDecodeJPEG(t *testing.T) {
	c := qt.New(t)

	tags := extractTags(t, "sunrise.jpg", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP)

	c.Assert(tags.EXIF()["Copyright"].Value, qt.Equals, "Bjørn Erik Pedersen")
	c.Assert(tags.EXIF()["ApertureValue"].Value, eq, 5.6)
	// c.Assert(tags.EXIF()["ThumbnailOffset"].Value, eq, 1338)
	c.Assert(tags.XMP()["CreatorTool"].Value, qt.Equals, "Adobe Photoshop Lightroom Classic 12.4 (Macintosh)")
	c.Assert(tags.IPTC()["City"].Value, qt.Equals, "Benalmádena")
}

func TestDecodeTIFF(t *testing.T) {
	c := qt.New(t)

	tags := extractTags(t, "sunrise.tif", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP)

	c.Assert(len(tags.XMP()), qt.Equals, 146)

	// TODO(bep) currently only XMP implemented.
	c.Assert(len(tags.EXIF()), qt.Equals, 0)
	c.Assert(len(tags.IPTC()), qt.Equals, 0)

	c.Assert(tags.XMP()["CreatorTool"].Value, qt.Equals, "Adobe Photoshop Lightroom Classic 12.4 (Macintosh)")
}

func TestDecodeCorrupt(t *testing.T) {
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
		c.Assert(imagemeta.IsInvalidFormat(err), qt.IsTrue, qt.Commentf("file: %s", file))
		img.Close()
	}
}

func TestDecodeCustomXMPHandler(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c, imagemeta.WebP)
	c.Cleanup(close)

	var xml string
	err := imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.WebP,
			HandleXMP: func(r io.Reader) error {
				b, err := io.ReadAll(r)
				xml = string(b)
				return err
			},
			Sources: imagemeta.XMP,
		},
	)

	c.Assert(err, qt.IsNil)
	c.Assert(xml, qt.Contains, "Sunrise in Spain")
}

func TestDecodeCustomXMPHandlerShortRead(t *testing.T) {
	c := qt.New(t)

	img, close := getSunrise(c, imagemeta.WebP)
	c.Cleanup(close)

	err := imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.WebP,
			HandleXMP: func(r io.Reader) error {
				return nil
			},
			Sources: imagemeta.XMP,
		},
	)

	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "expected EOF after XMP")
}

func TestDecodeShouldHandleTagEXIF(t *testing.T) {
	c := qt.New(t)

	const numTagsTotal = 64

	for i := 0; i < 30; i++ {
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

		err := imagemeta.Decode(
			imagemeta.Options{
				R:               img,
				ImageFormat:     imagemeta.JPEG,
				Sources:         imagemeta.EXIF,
				HandleTag:       handleTag,
				ShouldHandleTag: shouldHandle,
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

	img, err := os.Open(filepath.Join("testdata", filename))
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

	err = imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.JPEG,
			HandleTag:   handleTag,
			Sources:     imagemeta.IPTC,
		},
	)
	c.Assert(err, qt.IsNil)

	c.Assert(len(tags.IPTC()), qt.Equals, 22)
	// These hyphens looks odd, but it's how Exiftool has defined it.
	c.Assert(tags.IPTC()["By-line"].Value, qt.DeepEquals, []string{"Creator1 (ref2021.1)"})
	c.Assert(tags.IPTC()["By-lineTitle"].Value, qt.DeepEquals, []string{"Creator's Job Title  (ref2021.1)"})
	c.Assert(tags.IPTC()["DateCreated"].Value, qt.Equals, "20211020")
	c.Assert(tags.IPTC()["Keywords"].Value, qt.DeepEquals, []string{"Keyword1ref2021.1", "Keyword2ref2021.1", "Keyword3ref2021.1"})
}

func TestDecodeNamespace(t *testing.T) {
	c := qt.New(t)

	tags := extractTags(t, "sunrise.jpg", imagemeta.EXIF|imagemeta.IPTC|imagemeta.XMP)

	c.Assert(tags.EXIF()["Artist"].Namespace, qt.Equals, "IFD0")
	c.Assert(tags.EXIF()["GPSLatitude"].Namespace, qt.Equals, "IFD0/GPSInfoIFD")
	c.Assert(tags.EXIF()["Compression"].Namespace, qt.Equals, "IFD1")
	c.Assert(tags.IPTC()["City"].Namespace, qt.Equals, "IPTCApplication")
	c.Assert(tags.XMP()["AlreadyApplied"].Namespace, qt.Equals, "http://ns.adobe.com/camera-raw-settings/1.0/")
}

func TestDecodeOrientationOnly(t *testing.T) {
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

	err := imagemeta.Decode(
		imagemeta.Options{
			R:           img,
			ImageFormat: imagemeta.JPEG,
			HandleTag:   handleTag,
			Sources:     imagemeta.EXIF,
		},
	)

	c.Assert(err, qt.IsNil)
	c.Assert(tags.EXIF()["Orientation"].Value, qt.Equals, uint16(1))
	c.Assert(len(tags.EXIF()), qt.Equals, 1)
}

func TestDecodeXMPJPG(t *testing.T) {
	c := qt.New(t)

	tags := extractTags(t, "sunrise.jpg", imagemeta.XMP)

	c.Assert(len(tags.EXIF()) == 0, qt.IsTrue)
	c.Assert(len(tags.IPTC()) == 0, qt.IsTrue)
	c.Assert(len(tags.XMP()) > 0, qt.IsTrue)
}

func TestDecodeErrors(t *testing.T) {
	c := qt.New(t)

	c.Assert(imagemeta.Decode(imagemeta.Options{}), qt.ErrorMatches, "no reader provided")
	c.Assert(imagemeta.Decode(imagemeta.Options{R: strings.NewReader("foo")}), qt.ErrorMatches, "no image format provided.*")
	c.Assert(imagemeta.Decode(imagemeta.Options{R: strings.NewReader("foo"), ImageFormat: imagemeta.ImageFormat(1234)}), qt.ErrorMatches, "unsupported image format")
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

func TestGoldenXMP(t *testing.T) {
	// We do verify the "golden" tag count above, but ...
	t.Skip("XMP parsing is currently limited and the diff set is too large to reasoun about.")
	withGolden(t, imagemeta.XMP)
}

func TestGoldenTagCountEXIF(t *testing.T) {
	assertGoldenInfoTagCount(t, "IPTC-PhotometadataRef-Std2021.1.jpg", imagemeta.EXIF)
	assertGoldenInfoTagCount(t, "metadata_demo_exif_only.jpg", imagemeta.EXIF)
}

func TestGoldenTagCountIPTC(t *testing.T) {
	assertGoldenInfoTagCount(t, "metadata_demo_iim_and_xmp_only.jpg", imagemeta.IPTC)
}

func TestGoldenTagCountXMP(t *testing.T) {
	assertGoldenInfoTagCount(t, "sunrise.jpg", imagemeta.XMP)
}

func TestLatLong(t *testing.T) {
	c := qt.New(t)

	tags := extractTags(t, "sunrise.jpg", imagemeta.EXIF)

	lat, long, err := tags.GetLatLong()
	c.Assert(err, qt.IsNil)
	c.Assert(lat, eq, float64(36.59744166))
	c.Assert(long, eq, float64(-4.50846))

	tags = extractTags(t, "goexif_samples/geodegrees_as_string.jpg", imagemeta.EXIF)
	lat, long, err = tags.GetLatLong()
	c.Assert(err, qt.IsNil)
	c.Assert(lat, eq, float64(52.013888888))
	c.Assert(long, eq, float64(11.002777))
}

func TestGetDateTime(t *testing.T) {
	c := qt.New(t)

	tags := extractTags(t, "sunrise.jpg", imagemeta.EXIF)
	d, err := tags.GetDateTime()
	c.Assert(err, qt.IsNil)
	c.Assert(d.Format("2006-01-02"), qt.Equals, "2017-10-27")
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
}

func getSunrise(c *qt.C, imageFormat imagemeta.ImageFormat) (imagemeta.Reader, func()) {
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
	default:
		c.Fatalf("unknown image format: %v", imageFormat)
	}

	img, err := os.Open(filepath.Join("testdata", "sunrise"+ext))
	c.Assert(err, qt.IsNil)
	return img, func() {
		img.Close()
	}
}

func assertGoldenInfoTagCount(t testing.TB, filename string, sources imagemeta.TagSource) {
	c := qt.New(t)

	tags := extractTags(t, filename, sources)
	all := tags.All()

	// Our XMP parsing is currently a little limited so be a little lenient with the assertions.
	hasXMP := sources.Has(imagemeta.XMP)

	c.Assert(len(all) > 0, qt.IsTrue)

	goldenInfo := readGoldenInfo(t, filename)
	tagsLeft := make(map[string]imagemeta.TagInfo)
	tagsRight := make(map[string]any)

	if sources.Has(imagemeta.EXIF) {
		for k, v := range tags.EXIF() {
			tagsLeft[k] = v
		}
		for k, v := range goldenInfo.EXIF {
			tagsRight[k] = v
		}
	}
	if sources.Has(imagemeta.IPTC) {
		for k, v := range tags.IPTC() {
			tagsLeft[k] = v
		}
		for k, v := range goldenInfo.IPTC {
			tagsRight[k] = v
		}
	}
	if sources.Has(imagemeta.XMP) {
		for k, v := range tags.XMP() {
			tagsLeft[k] = v
		}
		for k, v := range goldenInfo.XMP {
			tagsRight[k] = v
		}
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

func compareWithExiftoolOutput(t testing.TB, filename string, sources imagemeta.TagSource) {
	c := qt.New(t)
	tags := extractTags(t, filename, sources)
	all := tags.All()
	tagsGolden := readGoldenInfo(t, filename)

	var tagsSorted []imagemeta.TagInfo
	for _, v := range all {
		tagsSorted = append(tagsSorted, v)
	}
	sort.Slice(tagsSorted, func(i, j int) bool {
		return tagsSorted[i].Tag < tagsSorted[j].Tag
	})

	xmpReplacer := strings.NewReplacer(
		"true", "True",
	)

	for _, v := range tagsSorted {
		normalizeUs := func(s string, our any) any {
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

		normalizeThem := func(s string, v any) any {
			if sources.Has(imagemeta.XMP) {
				// Our current XMP handling is very limited in the type department.
				// Convert v to a string.
				return xmpReplacer.Replace(fmt.Sprintf("%v", v))
			}
			switch v := v.(type) {
			case string:
				v = strings.TrimSpace(v)
				switch s {
				case "ShutterSpeedValue", "SubSecTimeDigitized", "SubSecTimeOriginal":
					f, _ := strconv.ParseFloat(v, 64)
					return f
				case "WhiteBalance":
					if strings.TrimSpace(v) == "AUTO1" {
						return float64(0)
					}
				}
				return v
			case float64:
				switch s {
				case "SerialNumber", "LensSerialNumber":
					return fmt.Sprintf("%d", int(v))
				}
			}
			return v
		}

		if exifToolValue, found := tagsGolden.EXIF[v.Tag]; found {
			// TODO1
			if v.Tag == "ThumbnailOffset" {
				continue
			}
			expect := normalizeThem(v.Tag, exifToolValue)
			got := normalizeUs(v.Tag, v.Value)

			c.Assert(got, eq, expect, qt.Commentf("%s: got: %T/%T  %v %q", v.Tag, got, expect, v.Value, filename))
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
	default:
		panic("unknown image format")
	}
}

func extractTags(t testing.TB, filename string, sources imagemeta.TagSource) imagemeta.Tags {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var tags imagemeta.Tags
	handleTag := func(ti imagemeta.TagInfo) error {
		if !tags.Has(ti) {
			tags.Add(ti)
		}
		return nil
	}
	shouldHandle := func(ti imagemeta.TagInfo) bool {
		return true
	}

	imageFormat := extToFormat(filepath.Ext(filename))

	err = imagemeta.Decode(imagemeta.Options{R: f, ImageFormat: imageFormat, ShouldHandleTag: shouldHandle, HandleTag: handleTag, Sources: sources})
	if err != nil {
		t.Fatal(fmt.Errorf("failed to decode %q: %w", filename, err))
	}
	return tags
}

func readGoldenInfo(t testing.TB, filename string) goldenFileInfo {
	exiftoolsJSONFilename := filepath.Join("gen", "testdata_exiftool", filename+".json")
	var exifToolValue []goldenFileInfo
	b, err := os.ReadFile(exiftoolsJSONFilename)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &exifToolValue); err != nil {
		t.Fatal(err)
	}
	m := exifToolValue[0]

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

func withGolden(t testing.TB, sources imagemeta.TagSource) {
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
	err := filepath.Walk("testdata", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		path = strings.TrimPrefix(path, "testdata"+string(filepath.Separator))

		return fn(path, info, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
}

var goldenSkip = map[string]bool{
	"goexif_samples/geodegrees_as_string.jpg": true, // The file has many EXIF errors. I think we do a better job than exiftools, but there are some differences.
}

var eq = qt.CmpEquals(
	cmp.Comparer(func(x, y imagemeta.Rat[uint32]) bool {
		return x.String() == y.String()
	}),

	cmp.Comparer(func(x, y imagemeta.Rat[int32]) bool {
		return x.String() == y.String()
	}),

	cmp.Comparer(func(x, y float64) bool {
		if x == y {
			return true
		}
		delta := math.Abs(x - y)
		mean := math.Abs(x+y) / 2.0
		return delta/mean < 0.00001
	}),
)

func BenchmarkDecode(b *testing.B) {
	handleTag := func(ti imagemeta.TagInfo) error {
		return nil
	}

	sourceSetEXIF := imagemeta.EXIF
	sourceSetIPTC := imagemeta.IPTC
	sourceSetAll := imagemeta.EXIF | imagemeta.IPTC | imagemeta.XMP

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

	imageFormat := imagemeta.PNG
	runBenchmark(b, "bep/imagemeta/png/exif", imagemeta.PNG, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetEXIF})
		return err
	})

	imageFormat = imagemeta.WebP
	runBenchmark(b, "bep/imagemeta/webp/all", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetAll})
		return err
	})

	runBenchmark(b, "bep/imagemeta/webp/xmp", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: imagemeta.XMP})
		return err
	})

	runBenchmark(b, "bep/imagemeta/webp/exif", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetEXIF})
		return err
	})

	imageFormat = imagemeta.JPEG
	runBenchmark(b, "bep/imagemeta/jpg/exif", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetEXIF})
		return err
	})

	runBenchmark(b, "bep/imagemeta/jpg/iptc", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetIPTC})
		return err
	})

	runBenchmark(b, "bep/imagemeta/jpg/xmp", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: imagemeta.XMP})
		return err
	})

	runBenchmark(b, "bep/imagemeta/jpg/all", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: imageFormat, HandleTag: handleTag, Sources: sourceSetAll})
		return err
	})
}

func BenchmarkDecodeExif(b *testing.B) {
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

	imageFormat := imagemeta.JPEG
	runBenchmark(b, "bep/imagemeta/exif/jpg/alltags", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{
			R: r, ImageFormat: imageFormat,

			HandleTag: func(ti imagemeta.TagInfo) error {
				return nil
			},
			Sources: imagemeta.EXIF,
		})
		return err
	})

	runBenchmark(b, "bep/imagemeta/exif/jpg/orientation", imageFormat, func(r imagemeta.Reader) error {
		err := imagemeta.Decode(imagemeta.Options{
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

	runBenchmark(b, "rwcarlsen/goexif/exif/jpg/alltags", imageFormat, func(r imagemeta.Reader) error {
		_, err := exif.Decode(r)
		return err
	})
}
