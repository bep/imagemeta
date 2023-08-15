package imagemeta

import (
	"encoding/xml"
	"errors"
	"io"
)

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
		return err
	}

	for _, attr := range meta.RDF.Description.Attrs {
		if attr.Name.Space == "xmlns" {
			continue
		}
		tagInfo := TagInfo{
			Source:    TagSourceXMP,
			Tag:       attr.Name.Local,
			Namespace: attr.Name.Space,
			Value:     attr.Value,
		}

		if err := opts.HandleTag(tagInfo); err != nil {
			return err
		}
	}
	return nil
}
