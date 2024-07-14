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
BenchmarkDecodeExif/bep/imagemeta/exif/jpg/alltags-10              64575             16883 ns/op            4288 B/op        161 allocs/op
BenchmarkDecodeExif/bep/imagemeta/exif/jpg/orientation-10         333732              3391 ns/op             409 B/op         10 allocs/op
BenchmarkDecodeExif/rwcarlsen/goexif/exif/jpg/alltags-10           24782             46589 ns/op          175552 B/op        812 allocs/op
```

Looking at some more extensive tests, testing different image formats and tag sources, we see that the current XMP implementation leaves a lot to be desired (you can provide your own XMP handler if you want). 

```bash
BenchmarkDecode/bep/imagemeta/png/exif-10                  23732             49936 ns/op            4300 B/op        162 allocs/op
BenchmarkDecode/bep/imagemeta/webp/all-10                   2970            391278 ns/op          177787 B/op       2430 allocs/op
BenchmarkDecode/bep/imagemeta/webp/xmp-10                   3183            369343 ns/op          139862 B/op       2265 allocs/op
BenchmarkDecode/bep/imagemeta/webp/exif-10                 38940             31184 ns/op           38075 B/op        171 allocs/op
BenchmarkDecode/bep/imagemeta/jpg/exif-10                  68695             17670 ns/op            4289 B/op        161 allocs/op
BenchmarkDecode/bep/imagemeta/jpg/iptc-10                 158425              7476 ns/op            1011 B/op         66 allocs/op
BenchmarkDecode/bep/imagemeta/jpg/xmp-10                    3115            367200 ns/op          139861 B/op       2264 allocs/op
BenchmarkDecode/bep/imagemeta/jpg/all-10                    3004            383872 ns/op          145157 B/op       2483 allocs/op
```

## When in doubt, Exiftool is right

The output of this library is tested against `exiftool -n -json`. This means, for example, that:

*  We use f-numbers and not APEX for aperture values.
*  We use seconds and not APEX for shutter speed values.
*  EXIF field definitions are fetched from this table:  https://exiftool.org/TagNames/EXIF.html
*  IPTC field definitions are fetched from this table:  https://exiftool.org/TagNames/IPTC.html
*  The XMP handling is currently very simple, you can supply your own XMP handler (see the `HandleXMP` option) if you need more.

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

Debugging tips:

```bash
 exiftool testdata/goexif_samples/has-lens-info.jpg -htmldump > dump.html
 ```