// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"math"
	"slices"
	"strings"
)

const (
	exifTagImageDescription   = 0x010E
	exifTagArtist             = 0x013B
	exifTagCopyright          = 0x8298
	exifTagExifIFDPointer     = 0x8769
	exifTagGPSIFDPointer      = 0x8825
	exifTagExifVersion        = 0x9000
	exifTagDateTimeOriginal   = 0x9003
	exifTagOffsetTimeOriginal = 0x9011

	gpsTagVersionID    = 0x0000
	gpsTagLatitudeRef  = 0x0001
	gpsTagLatitude     = 0x0002
	gpsTagLongitudeRef = 0x0003
	gpsTagLongitude    = 0x0004
)

type ifdEntry struct {
	tag   uint16
	typ   exifType
	count uint32
	value []byte // Encoded in the target byte order.
	raw   []byte // The 4 value/offset bytes of an existing entry, used as is.
}

func asciiEntry(tag uint16, s string) ifdEntry {
	b := append([]byte(strings.ReplaceAll(s, "\x00", "")), 0)
	return ifdEntry{tag: tag, typ: exifTypeASCIIString1, count: uint32(len(b)), value: b}
}

// exifEntries returns the IFD0, Exif IFD and GPS IFD entries for m, excluding the IFD pointers.
func (m Meta) exifEntries(bo byteOrder) (ifd0, exif, gps []ifdEntry) {
	if m.Description != "" {
		ifd0 = append(ifd0, asciiEntry(exifTagImageDescription, m.Description))
	}
	if m.Creator != "" {
		ifd0 = append(ifd0, asciiEntry(exifTagArtist, m.Creator))
	}
	if m.Copyright != "" {
		ifd0 = append(ifd0, asciiEntry(exifTagCopyright, m.Copyright))
	}
	if !m.Date.IsZero() {
		exif = append(exif,
			ifdEntry{tag: exifTagExifVersion, typ: exifTypeUndef1, count: 4, value: []byte("0232")},
			asciiEntry(exifTagDateTimeOriginal, m.Date.Format("2006:01:02 15:04:05")),
			asciiEntry(exifTagOffsetTimeOriginal, m.Date.Format("-07:00")),
		)
	}
	if ll := m.LatLong; ll != nil {
		ref := func(v float64, pos, neg string) string {
			if v < 0 {
				return neg
			}
			return pos
		}
		gps = append(gps,
			ifdEntry{tag: gpsTagVersionID, typ: exifTypeUnsignedByte1, count: 4, value: []byte{2, 3, 0, 0}},
			asciiEntry(gpsTagLatitudeRef, ref(ll.Lat, "N", "S")),
			gpsCoordEntry(bo, gpsTagLatitude, ll.Lat),
			asciiEntry(gpsTagLongitudeRef, ref(ll.Long, "E", "W")),
			gpsCoordEntry(bo, gpsTagLongitude, ll.Long),
		)
	}
	return
}

// gpsCoordEntry encodes v as degrees, minutes and seconds with microsecond precision.
func gpsCoordEntry(bo byteOrder, tag uint16, v float64) ifdEntry {
	const secDenom = 1_000_000
	total := uint64(math.Round(math.Abs(v) * 3600 * secDenom))
	deg := total / (3600 * secDenom)
	total -= deg * 3600 * secDenom
	minutes := total / (60 * secDenom)
	sec := total - minutes*60*secDenom

	var b []byte
	for _, r := range [][2]uint64{{deg, 1}, {minutes, 1}, {sec, secDenom}} {
		b = bo.AppendUint32(b, uint32(r[0]))
		b = bo.AppendUint32(b, uint32(r[1]))
	}
	return ifdEntry{tag: tag, typ: exifTypeUnsignedRat8, count: 3, value: b}
}

// encodeEXIF returns m's EXIF as a big endian TIFF structure, or nil if there's nothing to encode.
func (m Meta) encodeEXIF() []byte {
	bo := binary.BigEndian
	ifd0, exif, gps := m.exifEntries(bo)
	if len(ifd0)+len(exif)+len(gps) == 0 {
		return nil
	}
	b := make([]byte, 0, 8)
	b = bo.AppendUint16(b, tiffMarker.byteOrderBE)
	b = bo.AppendUint16(b, tiffMarker.meaningOfLife)
	b = bo.AppendUint32(b, 8)
	return encodeIFDs(b, bo, 8, ifd0, exif, gps, 0)
}

// encodeIFDs appends IFD0 and, if not empty, the Exif and GPS IFDs to buf.
// start is the offset of IFD0 relative to the TIFF header and must be even.
// next is the offset of the IFD following IFD0.
func encodeIFDs(buf []byte, bo byteOrder, start uint32, ifd0, exif, gps []ifdEntry, next uint32) []byte {
	ifdSize := func(entries []ifdEntry) uint32 {
		if len(entries) == 0 {
			return 0
		}
		n := uint32(6 + 12*len(entries))
		for _, e := range entries {
			if len(e.value) > 4 {
				n += uint32(len(e.value)+1) &^ 1
			}
		}
		return n
	}
	pointer := func(tag uint16) ifdEntry {
		return ifdEntry{tag: tag, typ: exifTypeUnsignedLong4, count: 1, value: make([]byte, 4)}
	}

	ifd0 = slices.Clone(ifd0)
	if len(exif) > 0 {
		ifd0 = append(ifd0, pointer(exifTagExifIFDPointer))
	}
	if len(gps) > 0 {
		ifd0 = append(ifd0, pointer(exifTagGPSIFDPointer))
	}
	for _, entries := range [][]ifdEntry{ifd0, exif, gps} {
		slices.SortFunc(entries, func(a, b ifdEntry) int { return int(a.tag) - int(b.tag) })
	}

	offExif := start + ifdSize(ifd0)
	offGPS := offExif + ifdSize(exif)
	for _, e := range ifd0 {
		switch e.tag {
		case exifTagExifIFDPointer:
			bo.PutUint32(e.value, offExif)
		case exifTagGPSIFDPointer:
			bo.PutUint32(e.value, offGPS)
		}
	}

	buf = appendIFD(buf, bo, start, ifd0, next)
	if len(exif) > 0 {
		buf = appendIFD(buf, bo, offExif, exif, 0)
	}
	if len(gps) > 0 {
		buf = appendIFD(buf, bo, offGPS, gps, 0)
	}
	return buf
}

func appendIFD(buf []byte, bo byteOrder, offset uint32, entries []ifdEntry, next uint32) []byte {
	dataOffset := offset + 6 + 12*uint32(len(entries))
	var data []byte
	buf = bo.AppendUint16(buf, uint16(len(entries)))
	for _, e := range entries {
		buf = bo.AppendUint16(buf, e.tag)
		buf = bo.AppendUint16(buf, uint16(e.typ))
		buf = bo.AppendUint32(buf, e.count)
		switch {
		case e.raw != nil:
			buf = append(buf, e.raw...)
		case len(e.value) <= 4:
			var v [4]byte
			copy(v[:], e.value)
			buf = append(buf, v[:]...)
		default:
			buf = bo.AppendUint32(buf, dataOffset+uint32(len(data)))
			data = append(data, e.value...)
			if len(data)%2 == 1 {
				data = append(data, 0)
			}
		}
	}
	buf = bo.AppendUint32(buf, next)
	return append(buf, data...)
}

// encodeXMP returns m as an XMP packet.
func (m Meta) encodeXMP() []byte {
	var sb strings.Builder
	esc := func(s string) {
		xml.EscapeText(&sb, []byte(s))
	}
	alt := func(name, s string) {
		if s == "" {
			return
		}
		fmt.Fprintf(&sb, "   <%s><rdf:Alt><rdf:li xml:lang=\"x-default\">", name)
		esc(s)
		fmt.Fprintf(&sb, "</rdf:li></rdf:Alt></%s>\n", name)
	}
	simple := func(name, s string) {
		fmt.Fprintf(&sb, "   <%s>", name)
		esc(s)
		fmt.Fprintf(&sb, "</%s>\n", name)
	}

	sb.WriteString(`<?xpacket begin="` + "\uFEFF" + `" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:exif="http://ns.adobe.com/exif/1.0/"
    xmlns:photoshop="http://ns.adobe.com/photoshop/1.0/">
`)
	alt("dc:rights", m.Copyright)
	if m.Creator != "" {
		sb.WriteString("   <dc:creator><rdf:Seq><rdf:li>")
		esc(m.Creator)
		sb.WriteString("</rdf:li></rdf:Seq></dc:creator>\n")
	}
	alt("dc:title", m.Title)
	alt("dc:description", m.Description)
	if !m.Date.IsZero() {
		simple("photoshop:DateCreated", m.Date.Format("2006-01-02T15:04:05-07:00"))
	}
	if ll := m.LatLong; ll != nil {
		simple("exif:GPSVersionID", "2.3.0.0")
		simple("exif:GPSLatitude", xmpGPSCoord(ll.Lat, "N", "S"))
		simple("exif:GPSLongitude", xmpGPSCoord(ll.Long, "E", "W"))
	}
	sb.WriteString(`  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`)

	return []byte(sb.String())
}

// xmpGPSCoord formats v as "DDD,MM.mmmmmmR".
func xmpGPSCoord(v float64, pos, neg string) string {
	ref := pos
	if v < 0 {
		ref = neg
		v = -v
	}
	total := math.Round(v * 60 * 1e6) // Micro minutes.
	deg := math.Floor(total / (60 * 1e6))
	minutes := (total - deg*60*1e6) / 1e6
	return fmt.Sprintf("%d,%.6f%s", int(deg), minutes, ref)
}

type byteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}
