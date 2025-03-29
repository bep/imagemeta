// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
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
		XMLName xml.Name
		Items   []string `xml:"li"`
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
			Tag:       attr.Name.Local,
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

	if err := processChildElements("creator", meta.RDF.Description.Creator.Seq.Items, opts, meta.RDF.Description.Creator.XMLName.Space); err != nil {
		return err
	}

	if err := processChildElements("publisher", meta.RDF.Description.Publisher.Bag.Items, opts, meta.RDF.Description.Publisher.XMLName.Space); err != nil {
		return err
	}

	if err := processChildElements("subject", meta.RDF.Description.Subject.Bag.Items, opts, meta.RDF.Description.Subject.XMLName.Space); err != nil {
		return err
	}

	if err := processChildElements("rights", meta.RDF.Description.Rights.Alt.Items, opts, meta.RDF.Description.Rights.XMLName.Space); err != nil {
		return err
	}

	return nil
}

func processChildElements(tag string, items []string, opts Options, namespace string) error {
	if len(items) == 0 {
		return nil
	}
	joined := strings.Join(items, ", ")
	tagInfo := TagInfo{
		Source:    XMP,
		Tag:       tag,
		Namespace: namespace,
		Value:     joined,
	}
	if !opts.ShouldHandleTag(tagInfo) {
		return nil
	}
	return opts.HandleTag(tagInfo)
}
