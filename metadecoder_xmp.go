package imagemeta

import "encoding/xml"

type rdf struct {
	Description rdfDescription `xml:"Description"`
}

type rdfDescription struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type xmpmeta struct {
	RDF rdf `xml:"RDF"`
}
