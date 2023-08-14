package imagemeta

import (
	"encoding/xml"
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

func decodeXMP(r io.Reader, handleTag HandleTagFunc) error {
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

		if err := handleTag(tagInfo); err != nil {
			return err
		}
	}
	return nil
}
