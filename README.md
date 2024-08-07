[![Tests on Linux, MacOS and Windows](https://github.com/bep/imagemeta/workflows/Test/badge.svg)](https://github.com/bep/imagemeta/actions?query=workflow:Test)
[![Go Report Card](https://goreportcard.com/badge/github.com/bep/imagemeta)](https://goreportcard.com/report/github.com/bep/imagemeta)
[![codecov](https://codecov.io/gh/bep/imagemeta/branch/main/graph/badge.svg)](https://codecov.io/gh/bep/imagemeta)
[![GoDoc](https://godoc.org/github.com/bep/imagemeta?status.svg)](https://godoc.org/github.com/bep/imagemeta)

## This is about READING image metadata

Writing is not supported, and never will.

I welcome PRs with fixes, but please raise an issue first if you want to add new features.

## Performance

Extracting `EXIF` performs well, ref. the benhcmark below. Note that you can get a significant boost if you only need a subset of the fields (e.g. only the `Orientation`). The last line is with the library that [Hugo](https://github.com/gohugoio/hugo) used before it was replaced with this.

```bash
BenchmarkDecodeCompareWithGoexif/bep/imagemeta/exif/jpeg/alltags-10                62474             19054 ns/op            4218 B/op        188 allocs/op
BenchmarkDecodeCompareWithGoexif/bep/imagemeta/exif/jpeg/orientation-10           309145              3723 ns/op             352 B/op          8 allocs/op
BenchmarkDecodeCompareWithGoexif/rwcarlsen/goexif/exif/jpg/alltags-10              21987             50195 ns/op          175548 B/op        812 allocs/op
```

Looking at some more extensive tests, testing different image formats and tag sources, we see that the current XMP implementation leaves a lot to be desired (you can provide your own XMP handler if you want). 

```bash
BenchmarkDecode/png/exif-10                39164             30783 ns/op            4231 B/op        189 allocs/op
BenchmarkDecode/png/all-10                  5617            206111 ns/op           48611 B/op        310 allocs/op
BenchmarkDecode/webp/all-10                 3069            379637 ns/op          144181 B/op       2450 allocs/op
BenchmarkDecode/webp/xmp-10                 3291            359133 ns/op          139991 B/op       2265 allocs/op
BenchmarkDecode/webp/exif-10               47028             25788 ns/op            4255 B/op        191 allocs/op
BenchmarkDecode/jpg/exif-10                58701             20216 ns/op            4223 B/op        188 allocs/op
BenchmarkDecode/jpg/iptc-10               135777              8725 ns/op            1562 B/op         80 allocs/op
BenchmarkDecode/jpg/iptc/category-10      215674              5393 ns/op             456 B/op         15 allocs/op
BenchmarkDecode/jpg/iptc/city-10          192067              6201 ns/op             553 B/op         17 allocs/op
BenchmarkDecode/jpg/xmp-10                  3244            359436 ns/op          139861 B/op       2263 allocs/op
BenchmarkDecode/jpg/all-10                  2874            389489 ns/op          145700 B/op       2523 allocs/op
BenchmarkDecode/tiff/exif-10                2065            566786 ns/op          214089 B/op        282 allocs/op
BenchmarkDecode/tiff/iptc-10               16761             71003 ns/op            2603 B/op        133 allocs/op
BenchmarkDecode/tiff/all-10                 1267            933321 ns/op          356878 B/op       2668 allocs/op
```

## When in doubt, Exiftool is right

The output of this library is tested against `exiftool -n -json`. This means, for example, that:

*  We use f-numbers and not APEX for aperture values.
*  We use seconds and not APEX for shutter speed values.
*  EXIF field definitions are fetched from this table:  https://exiftool.org/TagNames/EXIF.html
*  IPTC field definitions are fetched from this table:  https://exiftool.org/TagNames/IPTC.html
*  The XMP handling is currently very simple, you can supply your own XMP handler (see the `HandleXMP` option) if you need more.

There are some subtle differences in output:

* Exiftool prints rationale number arrays as space formatted strings with a format/precision that seems unnecessary hard to replicate, so we use `strconv.FormatFloat(f, 'f', -1, 64)` for these.

## Development

Many of the tests depends on generated golden files. To update these, run:

```bash
 go generate ./gen
```

Note that you need a working `exiftool` and updated binary in your `PATH` for this to work. This was tested OK with:

```
exiftool -ver
12.76
```

Debuggin tips:

```bash
 exiftool testdata/goexif_samples/has-lens-info.jpg -htmldump > dump.html
 ```
