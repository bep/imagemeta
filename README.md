[![Tests on Linux, MacOS and Windows](https://github.com/bep/imagemeta/workflows/Test/badge.svg)](https://github.com/bep/imagemeta/actions?query=workflow:Test)
[![Go Report Card](https://goreportcard.com/badge/github.com/bep/imagemeta)](https://goreportcard.com/report/github.com/bep/imagemeta)
[![codecov](https://codecov.io/gh/bep/imagemeta/branch/main/graph/badge.svg)](https://codecov.io/gh/bep/imagemeta)
[![GoDoc](https://godoc.org/github.com/bep/imagemeta?status.svg)](https://godoc.org/github.com/bep/imagemeta)

> [!CAUTION]
> This library is still a work in progress, and I would wait until it's merged into Hugo before consider using it or open issues/PRs about it.

## This is about READING image metadata

Writing is not supported, and never will.

I welcome PRs with fixes, but please raise an issue first if you want to add new features.

## Performance

Extracting `EXIF` performs well, ref. the benhcmark below. Note that you can get a significant boost if you only need a subset of the fields (e.g. only the `Orientation`). The last line is with the library that [Hugo](https://github.com/gohugoio/hugo) used before it was replaced with this.

```bash
BenchmarkDecodeCompareWithGoexif/bep/imagemeta/exif/jpeg/alltags-10                52466             21733 ns/op           12944 B/op        219 allocs/op
BenchmarkDecodeCompareWithGoexif/bep/imagemeta/exif/jpeg/orientation-10           253658              4861 ns/op            8548 B/op          9 allocs/op
BenchmarkDecodeCompareWithGoexif/rwcarlsen/goexif/exif/jpg/alltags-10              23415             47897 ns/op          175549 B/op        812 allocs/op
```

Looking at some more extensive tests, testing different image formats and tag sources, we see that the current XMP implementation leaves a lot to be desired (you can provide your own XMP handler if you want). 

```bash
BenchmarkDecode/png/exif-10                37803             31469 ns/op           12953 B/op        220 allocs/op
BenchmarkDecode/png/all-10                  5628            203294 ns/op           57296 B/op        341 allocs/op
BenchmarkDecode/webp/all-10                 3026            377070 ns/op          180064 B/op       2482 allocs/op
BenchmarkDecode/webp/xmp-10                 3199            353637 ns/op          167224 B/op       2266 allocs/op
BenchmarkDecode/webp/exif-10               45633             26524 ns/op           12977 B/op        222 allocs/op
BenchmarkDecode/jpg/exif-10                56980             20971 ns/op           12946 B/op        219 allocs/op
BenchmarkDecode/jpg/iptc-10               124027              9338 ns/op            8096 B/op         81 allocs/op
BenchmarkDecode/jpg/iptc/category-10              193405              6391 ns/op            6987 B/op         16 allocs/op
BenchmarkDecode/jpg/iptc/city-10                  170191              6757 ns/op            7083 B/op         18 allocs/op
BenchmarkDecode/jpg/xmp-10                          3201            353794 ns/op          139864 B/op       2263 allocs/op
BenchmarkDecode/jpg/all-10                          3032            381440 ns/op          160636 B/op       2555 allocs/op
BenchmarkDecode/tiff/exif-10                        2096            554931 ns/op          223802 B/op        319 allocs/op
BenchmarkDecode/tiff/iptc-10                       17203             69053 ns/op            2826 B/op        134 allocs/op
BenchmarkDecode/tiff/all-10                         1280            916291 ns/op          393300 B/op       2707 allocs/op
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