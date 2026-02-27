// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bep/imagemeta"
)

func FuzzDecodeJPG(f *testing.F) {
	filenames := []string{
		"bep/sunrise.jpg", "goexif/geodegrees_as_string.jpg",
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
	filenames := []string{"bep/sunrise.webp"}

	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}

	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.WebP)
	})
}

func FuzzDecodePNG(f *testing.F) {
	filenames := []string{"bep/sunrise.png"}

	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}

	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.PNG)
	})
}

func FuzzDecodeHEIF(f *testing.F) {
	filenames := []string{"iphone.heic", "sony.heif"}
	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}

	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.HEIF)
	})
}

func FuzzDecodeAVIF(f *testing.F) {
	// Use a HEIF file as seed corpus since we don't have a dedicated AVIF test image.
	filenames := []string{"iphone.heic"}
	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}

	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.AVIF)
	})
}

func FuzzDecodeTIFF(f *testing.F) {
	filenames := []string{"bep/sunrise.tif"}

	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}

	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.TIFF)
	})
}

func FuzzDecodeDNG(f *testing.F) {
	filenames := []string{"sample.dng"}
	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}
	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.DNG)
	})
}

func FuzzDecodeCR2(f *testing.F) {
	filenames := []string{"sample.cr2"}
	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}
	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.CR2)
	})
}

func FuzzDecodeNEF(f *testing.F) {
	filenames := []string{"sample.nef"}
	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}
	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.NEF)
	})
}

func FuzzDecodeARW(f *testing.F) {
	filenames := []string{"sample.arw"}
	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}
	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.ARW)
	})
}

func FuzzDecodePEF(f *testing.F) {
	filenames := []string{"bep/jølstravatnet.pef"}
	for _, filename := range filenames {
		f.Add(readTestDataFileAll(f, filename))
	}
	f.Fuzz(func(t *testing.T, imageBytes []byte) {
		fuzzDecodeBytes(t, imageBytes, imagemeta.PEF)
	})
}

func fuzzDecodeBytes(t *testing.T, imageBytes []byte, f imagemeta.ImageFormat) error {
	r := bytes.NewReader(imageBytes)
	_, err := imagemeta.Decode(imagemeta.Options{R: r, ImageFormat: f, Sources: imagemeta.EXIF | imagemeta.IPTC | imagemeta.XMP | imagemeta.CONFIG, Timeout: 600 * time.Millisecond})
	if err != nil {
		if !imagemeta.IsInvalidFormat(err) && !strings.Contains(err.Error(), "timed out") {
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
