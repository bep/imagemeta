// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/bep/imagemeta"
	qt "github.com/frankban/quicktest"
)

// TestDecodeHEIFMultipleXMP verifies that HEIC files with more than one
// application/rdf+xml item deliver each XMP packet separately (issue #67).
// Real Ultra HDR HEICs store two XMP items: one on the primary image and one
// on the gain map image. The test also inserts a non-XMP mime item to verify
// the content_type filter rejects anything other than application/rdf+xml.
func TestDecodeHEIFMultipleXMP(t *testing.T) {
	c := qt.New(t)

	xmpA := []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:GCamera="http://ns.google.com/photos/1.0/camera/" GCamera:MotionPhoto="1"/></rdf:RDF></x:xmpmeta>`)
	decoy := []byte("this is not xmp, must be rejected by content_type filter")
	xmpB := []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:hdrgm="http://ns.adobe.com/hdr-gain-map/1.0/" hdrgm:GainMapMax="2.3"/></rdf:RDF></x:xmpmeta>`)

	heif := buildMultiXMPHEIF(c, xmpA, decoy, xmpB)

	c.Run("HandleXMP receives each packet in order", func(c *qt.C) {
		var got [][]byte
		_, err := imagemeta.Decode(imagemeta.Options{
			R:           bytes.NewReader(heif),
			ImageFormat: imagemeta.HEIF,
			Sources:     imagemeta.XMP,
			HandleXMP: func(r io.Reader) error {
				b, err := io.ReadAll(r)
				if err != nil {
					return err
				}
				got = append(got, b)
				return nil
			},
			Warnf: panicWarnf,
		})
		c.Assert(err, qt.IsNil)
		c.Assert(got, qt.HasLen, 2)
		c.Assert(got[0], qt.DeepEquals, xmpA)
		c.Assert(got[1], qt.DeepEquals, xmpB)
		for _, b := range got {
			c.Assert(bytes.Contains(b, decoy), qt.IsFalse)
		}
	})

	c.Run("default XMP parser extracts tags from both packets", func(c *qt.C) {
		var tags imagemeta.Tags
		_, err := imagemeta.Decode(imagemeta.Options{
			R:           bytes.NewReader(heif),
			ImageFormat: imagemeta.HEIF,
			Sources:     imagemeta.XMP,
			HandleTag: func(ti imagemeta.TagInfo) error {
				tags.Add(ti)
				return nil
			},
			Warnf: panicWarnf,
		})
		c.Assert(err, qt.IsNil)
		all := tags.All()
		c.Assert(all["MotionPhoto"].Value, qt.Equals, "1")
		c.Assert(all["GainMapMax"].Value, qt.Equals, "2.3")
	})
}

// buildMultiXMPHEIF constructs a minimal ISOBMFF byte sequence modeling the
// Android Ultra HDR HEIC layout: ftyp + meta{iinf, iloc} + mdat. Three infe
// entries are emitted, all item_type "mime" — two with content_type
// application/rdf+xml and one with application/octet-stream (decoy).
func buildMultiXMPHEIF(c *qt.C, xmpA, decoy, xmpB []byte) []byte {
	c.Helper()

	box := func(typ string, body []byte) []byte {
		out := make([]byte, 8+len(body))
		binary.BigEndian.PutUint32(out[0:4], uint32(8+len(body)))
		copy(out[4:8], typ)
		copy(out[8:], body)
		return out
	}

	infe := func(id uint16, contentType string) []byte {
		var body bytes.Buffer
		body.Write([]byte{2, 0, 0, 0}) // version=2, flags=0
		binary.Write(&body, binary.BigEndian, id)
		body.Write([]byte{0, 0})       // item_protection_index
		body.WriteString("mime")       // item_type
		body.WriteByte(0)              // item_name ("")
		body.WriteString(contentType)
		body.WriteByte(0)              // content_type terminator
		return box("infe", body.Bytes())
	}

	// iinf (version 0): version+flags(4) + count(2) + entries.
	var iinfBody bytes.Buffer
	iinfBody.Write([]byte{0, 0, 0, 0})
	binary.Write(&iinfBody, binary.BigEndian, uint16(3))
	iinfBody.Write(infe(1, "application/rdf+xml"))
	iinfBody.Write(infe(2, "application/octet-stream"))
	iinfBody.Write(infe(3, "application/rdf+xml"))
	iinfBox := box("iinf", iinfBody.Bytes())

	// iloc (version 0): each entry is 2(id)+2(dref)+0(base)+2(extent_count)
	// +offset_size+length_size = 6+4+4 = 14 bytes.
	const ilocHeaderLen = 8 // 4 (v+f) + 0x44 + 0x00 + 2 (count)
	ilocBoxSize := 8 + ilocHeaderLen + 3*14

	ftypBody := []byte{'h', 'e', 'i', 'c', 0, 0, 0, 0, 'h', 'e', 'i', 'c'}
	ftypBox := box("ftyp", ftypBody)

	// meta FullBox: 4 bytes v+f + children (iinf, iloc).
	metaBodySize := 4 + len(iinfBox) + ilocBoxSize
	mdatBodyStart := len(ftypBox) + 8 + metaBodySize + 8 // +8 for meta and mdat headers

	off1 := uint32(mdatBodyStart)
	off2 := off1 + uint32(len(xmpA))
	off3 := off2 + uint32(len(decoy))

	ilocEntry := func(id uint16, offset, length uint32) []byte {
		b := make([]byte, 14)
		binary.BigEndian.PutUint16(b[0:2], id)
		binary.BigEndian.PutUint16(b[2:4], 0) // data_reference_index
		binary.BigEndian.PutUint16(b[4:6], 1) // extent_count
		binary.BigEndian.PutUint32(b[6:10], offset)
		binary.BigEndian.PutUint32(b[10:14], length)
		return b
	}

	var ilocBody bytes.Buffer
	ilocBody.Write([]byte{0, 0, 0, 0, 0x44, 0x00})
	binary.Write(&ilocBody, binary.BigEndian, uint16(3))
	ilocBody.Write(ilocEntry(1, off1, uint32(len(xmpA))))
	ilocBody.Write(ilocEntry(2, off2, uint32(len(decoy))))
	ilocBody.Write(ilocEntry(3, off3, uint32(len(xmpB))))
	ilocBox := box("iloc", ilocBody.Bytes())
	c.Assert(len(ilocBox), qt.Equals, ilocBoxSize)

	var metaBody bytes.Buffer
	metaBody.Write([]byte{0, 0, 0, 0})
	metaBody.Write(iinfBox)
	metaBody.Write(ilocBox)
	metaBox := box("meta", metaBody.Bytes())

	var mdatBody bytes.Buffer
	mdatBody.Write(xmpA)
	mdatBody.Write(decoy)
	mdatBody.Write(xmpB)
	mdatBox := box("mdat", mdatBody.Bytes())

	var out bytes.Buffer
	out.Write(ftypBox)
	out.Write(metaBox)
	out.Write(mdatBox)
	return out.Bytes()
}
