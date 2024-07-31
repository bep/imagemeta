// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

var xmpSkipNamespaces = map[string]bool{
	"xmlns": true,
	"http://www.w3.org/1999/02/22-rdf-syntax-ns#": true,
	"http://purl.org/dc/elements/1.1/":            true,
}

type rdf struct {
	Description rdfDescription `xml:"Description"`
}

type rdfDescription struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type xmpmeta struct {
	RDF rdf `xml:"RDF"`
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
	return nil
}
