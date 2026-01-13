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

var xmpSkipNamespaces = map[string]bool{
	"xmlns": true,
	"http://www.w3.org/1999/02/22-rdf-syntax-ns#": true,
	"http://purl.org/dc/elements/1.1/":            true,
}

type rdf struct {
	XMLName      xml.Name
	Descriptions []rdfDescription `xml:"Description"`
}

// Note: We currently only handle a subset of XMP tags,
// but a very common subset.
type rdfDescription struct {
	XMLName   xml.Name
	Attrs     []xml.Attr `xml:",any,attr"`
	Creator   seqList    `xml:"creator"`
	Publisher bagList    `xml:"publisher"`
	Subject   bagList    `xml:"subject"`
	Rights    altList    `xml:"rights"`

	// GPS and other simple child elements from exif namespace.
	GPSLatitude    string `xml:"GPSLatitude"`
	GPSLongitude   string `xml:"GPSLongitude"`
	GPSAltitude    string `xml:"GPSAltitude"`
	GPSAltitudeRef string `xml:"GPSAltitudeRef"`
}

type altList struct {
	XMLName xml.Name
	Alt     struct {
		Items []string `xml:"li"`
	} `xml:"Alt"`
}

type seqList struct {
	XMLName xml.Name
	Seq     struct {
		Items []string `xml:"li"`
	} `xml:"Seq"`
}

type bagList struct {
	XMLName xml.Name
	Bag     struct {
		Items []string `xml:"li"`
	} `xml:"Bag"`
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

	// Process all rdf:Description elements.
	for _, desc := range meta.RDF.Descriptions {
		// Process attributes.
		for _, attr := range desc.Attrs {
			if xmpSkipNamespaces[attr.Name.Space] {
				continue
			}

			tagInfo := TagInfo{
				Source:    XMP,
				Tag:       firstUpper(attr.Name.Local),
				Namespace: attr.Name.Space,
				Value:     attr.Value,
			}

			if !opts.ShouldHandleTag(tagInfo) {
				continue
			}

			if err := opts.HandleTag(tagInfo); err != nil {
				return err
			}
		}

		// Process known child element lists.
		if err := processChildElements(desc.Creator.XMLName, desc.Creator.Seq.Items, opts); err != nil {
			return err
		}

		if err := processChildElements(desc.Publisher.XMLName, desc.Publisher.Bag.Items, opts); err != nil {
			return err
		}

		if err := processChildElements(desc.Subject.XMLName, desc.Subject.Bag.Items, opts); err != nil {
			return err
		}

		if err := processChildElements(desc.Rights.XMLName, desc.Rights.Alt.Items, opts); err != nil {
			return err
		}

		// Process GPS child elements.
		// GPS coordinates in XMP are typically in DMS format like "26,34.951N"
		// which needs to be converted to decimal degrees.
		if desc.GPSLatitude != "" {
			if lat, err := parseXMPGPSCoordinate(desc.GPSLatitude); err == nil {
				if err := processGPSTag("GPSLatitude", lat, opts); err != nil {
					return err
				}
			}
		}

		if desc.GPSLongitude != "" {
			if long, err := parseXMPGPSCoordinate(desc.GPSLongitude); err == nil {
				if err := processGPSTag("GPSLongitude", long, opts); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func processChildElements(name xml.Name, items []string, opts Options) error {
	if len(items) == 0 {
		return nil
	}
	if name.Local == "" {
		return nil
	}
	var v any

	// This is how ExifTool does it:
	if len(items) == 1 {
		v = items[0]
	} else {
		v = items
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

// processGPSTag creates a TagInfo for a GPS coordinate and passes it to the handler.
func processGPSTag(tag string, value float64, opts Options) error {
	tagInfo := TagInfo{
		Source:    XMP,
		Tag:       tag,
		Namespace: "http://ns.adobe.com/exif/1.0/",
		Value:     value,
	}
	if !opts.ShouldHandleTag(tagInfo) {
		return nil
	}
	return opts.HandleTag(tagInfo)
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
	if idx := strings.Index(s, ","); idx != -1 {
		// Format: "degrees,minutes" e.g., "26,34.951"
		degStr := s[:idx]
		minStr := s[idx+1:]

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
