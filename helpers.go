// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// errInvalidFormat is used when the format is invalid.
var errInvalidFormat = &InvalidFormatError{errors.New("invalid format")}

// IsInvalidFormat reports whether the error was an InvalidFormatError.
func IsInvalidFormat(err error) bool {
	return errors.Is(err, errInvalidFormat)
}

// InvalidFormatError is used when the format is invalid.
type InvalidFormatError struct {
	Err error
}

func (e *InvalidFormatError) Error() string {
	return "invalid format: " + e.Err.Error()
}

// Is reports whether the target error is an InvalidFormatError.
func (e *InvalidFormatError) Is(target error) bool {
	_, ok := target.(*InvalidFormatError)
	return ok
}

func newInvalidFormatErrorf(format string, args ...any) error {
	return &InvalidFormatError{fmt.Errorf(format, args...)}
}

func newInvalidFormatError(err error) error {
	return &InvalidFormatError{err}
}

// These error situations comes from the Go Fuzz modifying the input data to trigger panics.
// We want to separate panics that we can do something about and "invalid format" errors.
var invalidFormatErrorStrings = []string{
	"unexpected EOF",
}

func isInvalidFormatErrorCandidate(err error) bool {
	for _, s := range invalidFormatErrorStrings {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}

// Rat is a rational number.
type Rat[T int32 | uint32] interface {
	Num() T
	Den() T
	Float64() float64

	// String returns the string representation of the rational number.
	// If the denominator is 1, the string will be the numerator only.
	String() string
}

var (
	_ encoding.TextUnmarshaler = (*rat[int32])(nil)
	_ encoding.TextMarshaler   = rat[int32]{}
)

// rat is a rational number.
// It's a lightweight version of math/big.rat.
type rat[T int32 | uint32] struct {
	num T
	den T
}

// Num returns the numerator of the rational number.
func (r rat[T]) Num() T {
	return r.num
}

// Den returns the denominator of the rational number.
func (r rat[T]) Den() T {
	return r.den
}

// Float64 returns the float64 representation of the rational number.
func (r rat[T]) Float64() float64 {
	return float64(r.num) / float64(r.den)
}

// String returns the string representation of the rational number.
// If the denominator is 1, the string will be the numerator only.
func (r rat[T]) String() string {
	if r.den == 1 {
		return fmt.Sprintf("%d", r.num)
	}
	return fmt.Sprintf("%d/%d", r.num, r.den)
}

func (r rat[T]) Format(w fmt.State, v rune) {
	switch v {
	case 'f':
		fmt.Fprintf(w, "%f", r.Float64())
	default:
		fmt.Fprintf(w, "%s", r.String())
	}
}

func (r *rat[T]) UnmarshalText(text []byte) error {
	s := string(text)
	if !strings.Contains(s, "/") {
		num, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("failed to parse %q as a rational number: %w", s, err)
		}
		r.num = T(num)
		r.den = 1
		return nil
	}
	if _, err := fmt.Sscanf(s, "%d/%d", &r.num, &r.den); err != nil {
		return fmt.Errorf("failed to parse %q as a rational number: %w", s, err)
	}
	return nil
}

func (r rat[T]) MarshalText() (text []byte, err error) {
	return []byte(r.String()), nil
}

// NewRat returns a new Rat with the given numerator and denominator.
func NewRat[T int32 | uint32](num, den T) (Rat[T], error) {
	if den == 0 {
		return nil, fmt.Errorf("denominator must be non-zero")
	}

	// Remove the greatest common divisor.
	gcd := func(a, b T) T {
		for b != 0 {
			a, b = b, a%b
		}
		return a
	}
	d := gcd(num, den)
	if d != 1 {
		num, den = num/d, den/d
	}

	// Denominator must be positive.
	if den < 0 {
		num, den = -num, -den
	}

	return &rat[T]{num: num, den: den}, nil
}

type vc struct{}

func isUndefined(f float64) bool {
	return math.IsNaN(f) || math.IsInf(f, 0)
}

type float64Provider interface {
	Float64() float64
}

func (vc) convertAPEXToFNumber(ctx valueConverterContext, v any) any {
	r, ok := v.(float64Provider)
	if !ok {
		return 0
	}
	f := r.Float64()
	return math.Pow(2, f/2)
}

func (vc) convertAPEXToSeconds(ctx valueConverterContext, v any) any {
	r, ok := v.(float64Provider)
	if !ok {
		return 0
	}
	f := r.Float64()
	f = 1 / math.Pow(2, f)
	return f
}

func (c vc) convertBytesToStringDelimBy(ctx valueConverterContext, v any, delim string) any {
	bb, ok := typeAssertSlice[byte](ctx, v)
	if !ok {
		return ""
	}
	var buff bytes.Buffer
	for i, b := range bb {
		if i > 0 {
			buff.WriteString(delim)
		}
		buff.WriteString(strconv.Itoa(int(b)))
	}
	return buff.String()
}

func (c vc) convertBytesToStringSpaceDelim(ctx valueConverterContext, v any) any {
	return c.convertBytesToStringDelimBy(ctx, v, " ")
}

func (c vc) convertDegreesToDecimal(ctx valueConverterContext, v any) any {
	d, err := c.toDegrees(v)
	if err != nil {
		ctx.warnf("failed to convert degrees to decimal: %v", err)
		return 0.0

	}
	return d
}

func (vc) convertNumbersToSpaceLimited(ctx valueConverterContext, v any) any {
	nums, ok := typeAssertSlice[any](ctx, v)
	if !ok {
		return ""
	}

	var sb strings.Builder
	for i, n := range nums {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(fmt.Sprintf("%d", n))
	}
	return sb.String()
}

func (c vc) convertBinaryData(ctx valueConverterContext, v any) any {
	b, ok := typeAssert[[]byte](ctx, v)
	if !ok {
		return ""
	}
	return fmt.Sprintf("(Binary data %d bytes)", len(b))
}

func (c vc) convertRatsToSpaceLimited(ctx valueConverterContext, v any) any {
	nums, ok := typeAssert[[]any](ctx, v)
	if !ok {
		return ""
	}

	var sb strings.Builder
	for i, n := range nums {
		if i > 0 {
			sb.WriteString(" ")
		}
		var s string
		var f float64
		switch n := n.(type) {
		case string:
			s = n
		case float64Provider:
			f = n.Float64()
		case float64:
			f = n
		}

		if s == "" {
			if isUndefined(f) {
				s = undef
			} else {
				s = strconv.FormatFloat(f, 'f', -1, 64)
			}
		}

		sb.WriteString(s)
	}
	return sb.String()
}

func (vc) convertStringToInt(ctx valueConverterContext, v any) any {
	s, ok := typeAssert[string](ctx, v)
	if !ok {
		return 0
	}
	s = printableString(s)
	i, _ := strconv.Atoi(s)
	return i
}

func (c vc) convertUserComment(ctx valueConverterContext, v any) any {
	// UserComment tag is identified based on an ID code in a fixed 8-byte area at the start of the tag data area.
	b, ok := typeAssert[[]byte](ctx, v)
	if !ok {
		// Handle plain string user comment (which is against spec; but commonly done)
		// Exiftool prints a warning but returns the string as-is.
		// See https://github.com/exiftool/exiftool/blob/13.27/lib/Image/ExifTool/Exif.pm#L5483
		if text, ok := typeAssert[string](ctx, v); ok {
			return text
		}
		return ""
	}
	if len(b) < 8 {
		return ""
	}
	id := string(b[:8])

	switch id {
	case "ASCII\x00\x00\x00":
		s := printableString(string(trimBytesNulls(b[8:])))
		if !isASCII(s) {
			return ""
		}
		return s
	case "UNICODE\x00":
		return printableString(string(trimBytesNulls(b[8:])))
	case "\x00\x00\x00\x00\x00\x00\x00\x00":
		s := string(trimBytesNulls(b[8:]))
		if !utf8.ValidString(s) {
			return ""
		}
		return strings.TrimRight(s, " ")
	default:
		return ""
	}
}

func (vc) ratNum(v any) any {
	switch vv := v.(type) {
	case Rat[uint32]:
		return vv.Num()
	case Rat[int32]:
		return vv.Num()
	default:
		return 0
	}
}

func (c vc) convertToTimestampString(ctx valueConverterContext, v any) any {
	switch vv := v.(type) {
	case []any:
		if len(vv) != 3 {
			return time.Time{}
		}
		for i, v := range vv {
			vv[i] = c.ratNum(v)
		}
		s := fmt.Sprintf("%02d:%02d:%02d", vv...)

		if len(s) == 10 {
			// 13:03:4279 => 13:03:42.79
			s = s[:8] + "." + s[8:]
		}
		return s
	case string:
		// 17,00000,8,00000,29,0000
		parts := strings.Split(vv, ",")

		if len(parts) != 6 {
			return ""
		}
		var vvv []any
		for i := 0; i < 6; i += 2 {
			v, _ := strconv.Atoi(parts[i])
			vvv = append(vvv, v)
		}
		return fmt.Sprintf("%02d:%02d:%02d", vvv...)

	default:
		return ""
	}
}

func (vc) parseDegrees(s string) (float64, error) {
	if s == "" || s == "0100" {
		return 0, nil
	}
	var deg, min, sec float64
	_, err := fmt.Sscanf(s, "%f,%f,%f", &deg, &min, &sec)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %q: %w", s, err)
	}
	return deg + min/60 + sec/3600, nil
}

func (c vc) toDegrees(v any) (float64, error) {
	switch v := v.(type) {
	case []any:
		if len(v) != 3 {
			return 0.0, fmt.Errorf("expected 3 values, got %d", len(v))
		}

		deg := toFloat64(v[0])
		min := toFloat64(v[1])
		sec := toFloat64(v[2])

		return deg + min/60 + sec/3600, nil
	case float64:
		return v, nil
	case string:
		return c.parseDegrees(v)
	case []byte:
		return c.parseDegrees(string(v))
	default:
		return 0.0, fmt.Errorf("unsupported degree type %T", v)
	}
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func printableString(s string) string {
	ss := strings.Map(func(r rune) rune {
		if unicode.IsGraphic(r) {
			return r
		}
		return -1
	}, s)

	return strings.TrimSpace(ss)
}

func toPrintableValue(v any) any {
	switch vv := v.(type) {
	case string:
		return printableString(vv)
	case []byte:
		return printableString(string(trimBytesNulls(vv)))
	default:
		return v
	}
}

func toFloat64(v any) float64 {
	switch vv := v.(type) {
	case float64Provider:
		return vv.Float64()
	case float64:
		return vv
	default:
		return 0
	}
}

func toString(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	case []byte:
		return string(trimBytesNulls(vv))
	default:
		return fmt.Sprintf("%v", vv)
	}
}

func trimBytesNulls(b []byte) []byte {
	var lo, hi int
	for lo = 0; lo < len(b) && b[lo] == 0; lo++ {
	}
	for hi = len(b) - 1; hi >= 0 && b[hi] == 0; hi-- {
	}
	if lo > hi {
		return nil
	}
	return b[lo : hi+1]
}

func typeAssertSlice[T any](ctx valueConverterContext, v any) ([]T, bool) {
	vv, ok := v.([]T)
	if ok {
		return vv, true
	}

	vvv, ok := v.(T)
	if ok {
		return []T{vvv}, true
	}

	ctx.warnf("expected %T or %T, got %T", vv, vvv, v)

	return vv, false
}

func typeAssert[T any](ctx valueConverterContext, v any) (T, bool) {
	vv, ok := v.(T)
	if !ok {
		ctx.warnf("expected %T, got %T", vv, v)
		return vv, false
	}
	return vv, true
}
