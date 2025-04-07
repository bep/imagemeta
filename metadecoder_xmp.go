// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"unicode"
	"unicode/utf8"
)

var xmpSkipNamespaces = map[string]bool{
	"xmlns": true,
	"http://www.w3.org/1999/02/22-rdf-syntax-ns#": true,
	"http://purl.org/dc/elements/1.1/":            true,
}

type rdf struct {
	XMLName     xml.Name
	Description rdfDescription `xml:"Description"`
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

	for _, attr := range meta.RDF.Description.Attrs {
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

	if err := processChildElements(meta.RDF.Description.Creator.XMLName, meta.RDF.Description.Creator.Seq.Items, opts); err != nil {
		return err
	}

	if err := processChildElements(meta.RDF.Description.Publisher.XMLName, meta.RDF.Description.Publisher.Bag.Items, opts); err != nil {
		return err
	}

	if err := processChildElements(meta.RDF.Description.Subject.XMLName, meta.RDF.Description.Subject.Bag.Items, opts); err != nil {
		return err
	}

	if err := processChildElements(meta.RDF.Description.Rights.XMLName, meta.RDF.Description.Rights.Alt.Items, opts); err != nil {
		return err
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
