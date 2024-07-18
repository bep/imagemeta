package imagemeta

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

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
func NewRat[T int32 | uint32](num, den T) Rat[T] {
	if den == 0 {
		panic("division by zero")
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

	return &rat[T]{num: num, den: den}
}

type vc struct{}

func (vc) isUndefined(f float64) bool {
	return math.IsNaN(f) || math.IsInf(f, 0)
}

type float64Provider interface {
	Float64() float64
}

func (vc) convertAPEXToFNumber(byteOrder binary.ByteOrder, v any) any {
	r, ok := v.(float64Provider)
	if !ok {
		return 0
	}
	f := r.Float64()
	return math.Pow(2, f/2)
}

func (vc) convertAPEXToSeconds(byteOrder binary.ByteOrder, v any) any {
	r, ok := v.(float64Provider)
	if !ok {
		return 0
	}
	f := r.Float64()
	f = 1 / math.Pow(2, f)
	return f
}

func (c vc) convertBytesToStringDelimBy(v any, delim string) any {
	bb := v.([]byte)
	var buff bytes.Buffer
	for i, b := range bb {
		if i > 0 {
			buff.WriteString(delim)
		}
		buff.WriteString(strconv.Itoa(int(b)))
	}
	return buff.String()
}

func (c vc) convertBytesToStringSpaceDelim(byteOrder binary.ByteOrder, v any) any {
	return c.convertBytesToStringDelimBy(v, " ")
}

func (c vc) convertDegreesToDecimal(byteOrder binary.ByteOrder, v any) any {
	d, _ := c.toDegrees(v)
	return d
}

func (vc) convertNumbersToSpaceLimited(byteOrder binary.ByteOrder, v any) any {
	var sb strings.Builder
	nums := v.([]any)
	for i, n := range nums {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(fmt.Sprintf("%d", n))
	}
	return sb.String()
}

func (c vc) convertBinaryData(byteOrder binary.ByteOrder, v any) any {
	b := v.([]byte)
	return fmt.Sprintf("(Binary data %d bytes)", len(b))
}

func (c vc) convertRatsToSpaceLimited(byteOrder binary.ByteOrder, v any) any {
	nums := v.([]any)
	var sb strings.Builder
	for i, n := range nums {
		if i > 0 {
			sb.WriteString(" ")
		}
		var f float64
		switch n := n.(type) {
		case float64Provider:
			f = n.Float64()
		case float64:
			f = n
		}
		var s string
		if c.isUndefined(f) {
			s = "undef"
		} else {
			s = strconv.FormatFloat(f, 'f', -1, 64)
		}

		sb.WriteString(s)
	}
	return sb.String()
}

func (vc) convertStringToInt(byteOrder binary.ByteOrder, v any) any {
	s := printableString(v.(string))
	i, _ := strconv.Atoi(s)
	return i
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

func (c vc) convertToTimestampString(byteOrder binary.ByteOrder, v any) any {
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
	default:
		// TODO1: Other types, test.
		return 0.0, fmt.Errorf("unsupported degree type %T", v)
	}
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
