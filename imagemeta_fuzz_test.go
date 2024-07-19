package imagemeta_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bep/imagemeta"
)

func FuzzDecodeJPG(f *testing.F) {
	filenames := []string{
		"sunrise.jpg", "goexif/geodegrees_as_string.jpg",
		"metadata_demo_exif_only.jpg", "metadata_demo_iim_and_xmp_only.jpg",
		"corrupt/infinite_loop_exif.jpg",
		"corrupt/max_uint32_exif.jpg",
	}
	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}

	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.JPEG)
	})
}

func FuzzDecodeWebP(f *testing.F) {
	filenames := []string{"sunrise.webp"}

	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}

	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.WebP)
	})
}

func FuzzDecodePNG(f *testing.F) {
	filenames := []string{"sunrise.png", "metadata-extractor-images/png/issue614.png"}

	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}

	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.PNG)
	})
}

func FuzzDecodeTIFF(f *testing.F) {
	filenames := []string{"sunrise.tif"}

	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}

	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.TIFF)
	})
}

func fuzzDecodeBytes(t *testing.T, imageBytes []byte, f imagemeta.ImageFormat) error {
	r := bytes.NewReader(imageBytes)
	err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: f, Sources: imagemeta.EXIF | imagemeta.IPTC | imagemeta.XMP})
	if err != nil {
		if !imagemeta.IsInvalidFormat(err) {
			t.Fatalf("unknown error in Decode: %v %T", err, err)
		}
	}
	return nil
}

func readTestDataFileAll(t testing.TB, filename string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "images", filename))
	if err != nil {
		t.Fatalf("failed to read file %q: %v", filename, err)
	}
	return b
}
