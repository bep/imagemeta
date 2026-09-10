// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const rdfNamespace = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"

var xmpSkipNamespaces = map[string]bool{
	"xmlns":      true,
	rdfNamespace: true,
}

type rdf struct {
	XMLName      xml.Name
	Descriptions []rdfDescription `xml:"Description"`
}

type rdfDescription struct {
	XMLName xml.Name
	Attrs   []xml.Attr   `xml:",any,attr"`
	Elems   []rdfElement `xml:",any"`
}

// rdfElement is a property of an rdf:Description written in element form:
// a simple text value, an rdf:resource reference or an rdf:Seq/Bag/Alt list.
// Structs (nested rdf:Description or rdf:parseType="Resource") are not handled.
type rdfElement struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Text    string     `xml:",chardata"`
	Seq     *rdfList   `xml:"Seq"`
	Bag     *rdfList   `xml:"Bag"`
	Alt     *rdfList   `xml:"Alt"`
}

type rdfList struct {
	Items []string `xml:"li"`
}

// value returns the list items as ExifTool does: a single item as a string,
// several as a slice, none as nil.
func (l *rdfList) value() any {
	var items []string
	for _, item := range l.Items {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	switch len(items) {
	case 0:
		return nil
	case 1:
		return items[0]
	default:
		return items
	}
}

func (el rdfElement) value() any {
	switch {
	case el.Seq != nil:
		return el.Seq.value()
	case el.Bag != nil:
		return el.Bag.value()
	case el.Alt != nil:
		return el.Alt.value()
	}
	if s := strings.TrimSpace(el.Text); s != "" {
		return s
	}
	for _, attr := range el.Attrs {
		if attr.Name.Space == rdfNamespace && attr.Name.Local == "resource" {
			return attr.Value
		}
	}
	return nil
}

type xmpmeta struct {
	XMLName xml.Name
	RDF     rdf `xml:"RDF"`
}

func decodeXMP(r io.Reader, opts Options) error {
	if opts.HandleXMP != nil {
		if err := opts.HandleXMP(r); err != nil {
			return err
		}
		// Read one more byte to make sure we're at EOF.
		var b [1]byte
		if _, err := r.Read(b[:]); err != io.EOF {
			return errors.New("expected EOF after XMP")
		}
		return nil
	}

	var meta xmpmeta
	if err := xml.NewDecoder(r).Decode(&meta); err != nil {
		return newInvalidFormatError(fmt.Errorf("decoding XMP: %w", err))
	}

	for _, desc := range meta.RDF.Descriptions {
		for _, attr := range desc.Attrs {
			if err := handleXMPTag(attr.Name, attr.Value, opts); err != nil {
				return err
			}
		}
		for _, el := range desc.Elems {
			if err := handleXMPTag(el.XMLName, el.value(), opts); err != nil {
				return err
			}
		}
	}

	return nil
}

func handleXMPTag(name xml.Name, v any, opts Options) error {
	if v == nil || name.Local == "" || xmpSkipNamespaces[name.Space] {
		return nil
	}

	// GPS coordinates in XMP are typically in DMS format like "26,34.951N"
	// which needs to be converted to decimal degrees.
	switch name.Local {
	case "GPSLatitude", "GPSLongitude":
		s, ok := v.(string)
		if !ok {
			return nil
		}
		f, err := parseXMPGPSCoordinate(s)
		if err != nil {
			return nil
		}
		v = f
	}

	tagInfo := TagInfo{
		Source:    XMP,
		Tag:       firstUpper(name.Local),
		Namespace: name.Space,
		Value:     v,
	}
	if !opts.ShouldHandleTag(tagInfo) {
		return nil
	}
	return opts.HandleTag(tagInfo)
}

func firstUpper(s string) string {
	if s == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

// parseXMPGPSCoordinate parses GPS coordinates from XMP format.
// XMP GPS coordinates can be in several formats:
// - DMS with direction: "26,34.951N" or "80,12.014W"
// - Decimal with direction: "26.5825N" or "80.2002W"
// - Pure decimal: "26.5825" or "-80.2002"
func parseXMPGPSCoordinate(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty coordinate")
	}

	// Check for direction suffix (N, S, E, W)
	var negative bool
	lastChar := s[len(s)-1]
	switch lastChar {
	case 'S', 's', 'W', 'w':
		negative = true
		s = s[:len(s)-1]
	case 'N', 'n', 'E', 'e':
		s = s[:len(s)-1]
	}

	var degrees float64

	// Check if it's in DMS format (contains comma)
	if before, after, ok := strings.Cut(s, ","); ok {
		// Format: "degrees,minutes" e.g., "26,34.951"
		degStr := before
		minStr := after

		deg, err := strconv.ParseFloat(degStr, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing degrees: %w", err)
		}

		min, err := strconv.ParseFloat(minStr, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing minutes: %w", err)
		}

		degrees = deg + min/60.0
	} else {
		// Pure decimal format
		var err error
		degrees, err = strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing decimal: %w", err)
		}
	}

	if negative {
		degrees = -degrees
	}

	return degrees, nil
}
