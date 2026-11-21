// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"slices"
)

type imageEncoderAVIF struct {
	*baseEncoder
}

type isoBox struct {
	typ        fourCC
	start, end int
	hdr        int
	sizeToEOF  bool // The box's size field is 0.
}

func (bx isoBox) payload(b []byte) []byte {
	return b[bx.start+bx.hdr : bx.end]
}

func readISOBoxes(b []byte, pos, end int) ([]isoBox, error) {
	s := newSliceReader(b[:end], binary.BigEndian)
	s.seek(pos)
	var boxes []isoBox
	for s.remaining() > 0 {
		start := s.pos
		if s.remaining() < 8 {
			return nil, newInvalidFormatErrorf("avif: truncated box at offset %d", start)
		}
		size := uint64(s.read4())
		bx := isoBox{typ: s.readFourCC(), start: start, hdr: 8}
		switch size {
		case 0:
			size = uint64(end - start)
			bx.sizeToEOF = true
		case 1:
			if s.remaining() < 8 {
				return nil, newInvalidFormatErrorf("avif: truncated box at offset %d", start)
			}
			size = s.read8()
			bx.hdr = 16
		}
		if size < uint64(bx.hdr) || size > uint64(end-start) {
			return nil, newInvalidFormatErrorf("avif: invalid size of box %q at offset %d", bx.typ, start)
		}
		bx.end = start + int(size)
		boxes = append(boxes, bx)
		s.seek(bx.end)
	}
	return boxes, nil
}

func appendISOBox(buf []byte, typ fourCC, payload ...[]byte) []byte {
	n := 8
	for _, p := range payload {
		n += len(p)
	}
	buf = binary.BigEndian.AppendUint32(buf, uint32(n))
	buf = append(buf, typ[:]...)
	for _, p := range payload {
		buf = append(buf, p...)
	}
	return buf
}

// fixLastChildSizeToEOF ensures the last box in raw (with offsets relative to base)
// has an explicit size field before appending sibling boxes after it.
func fixLastChildSizeToEOF(raw []byte, base int, boxes []isoBox) ([]byte, error) {
	if len(boxes) == 0 {
		return raw, nil
	}
	last := boxes[len(boxes)-1]
	if !last.sizeToEOF {
		return raw, nil
	}
	if last.hdr != 8 || last.end-last.start > math.MaxUint32 {
		return nil, errors.New("avif: cannot fix size of last child box")
	}
	out := slices.Clone(raw)
	binary.BigEndian.PutUint32(out[last.start-base:], uint32(last.end-last.start))
	return out, nil
}

func (s *sliceReader) readVarUint(size int) uint64 {
	switch size {
	case 0:
		return 0
	case 4:
		return uint64(s.read4())
	case 8:
		return s.read8()
	}
	if s.err == nil {
		s.err = newInvalidFormatErrorf("avif: unsupported field size %d", size)
	}
	return 0
}

func (s *sliceReader) readID(wide bool) uint32 {
	if wide {
		return s.read4()
	}
	return uint32(s.read2())
}

func appendUint(buf []byte, size int, v uint64) []byte {
	switch size {
	case 4:
		return binary.BigEndian.AppendUint32(buf, uint32(v))
	case 8:
		return binary.BigEndian.AppendUint64(buf, v)
	}
	return buf
}

func appendID(buf []byte, wide bool, id uint32) []byte {
	if wide {
		return binary.BigEndian.AppendUint32(buf, id)
	}
	return binary.BigEndian.AppendUint16(buf, uint16(id))
}

func validIlocFieldSize(size int) bool {
	return size == 0 || size == 4 || size == 8
}

type ilocExtent struct {
	index, offset, length uint64
}

type ilocItem struct {
	id      uint32
	method  uint8
	dataRef uint16
	base    uint64
	extents []ilocExtent
}

type iloc struct {
	version                             uint8
	flags                               [3]byte
	offSize, lenSize, baseSize, idxSize int
	items                               []ilocItem
}

func parseIloc(p []byte) (*iloc, error) {
	c := newSliceReader(p, binary.BigEndian)
	l := &iloc{version: c.read1()}
	copy(l.flags[:], c.readBytes(3))
	if l.version > 2 {
		return nil, newInvalidFormatErrorf("avif: unsupported iloc version %d", l.version)
	}
	v := c.read1()
	l.offSize, l.lenSize = int(v>>4), int(v&0xf)
	v = c.read1()
	l.baseSize = int(v >> 4)
	if l.version > 0 {
		l.idxSize = int(v & 0xf)
	}
	if !validIlocFieldSize(l.offSize) || !validIlocFieldSize(l.lenSize) || !validIlocFieldSize(l.baseSize) || !validIlocFieldSize(l.idxSize) {
		return nil, newInvalidFormatErrorf("avif: unsupported iloc field sizes")
	}
	count := c.readID(l.version == 2)
	if c.err != nil {
		return nil, c.err
	}

	minItemSize := uint64(2 + 2 + l.baseSize + 2)
	if l.version == 2 {
		minItemSize += 2 // 32-bit item_ID
	}
	if l.version > 0 {
		minItemSize += 2 // construction_method
	}
	if uint64(count) > uint64(c.remaining())/minItemSize {
		return nil, newInvalidFormatErrorf("avif: invalid iloc item count %d", count)
	}

	extentSize := uint64(l.offSize + l.lenSize)
	if l.version > 0 {
		extentSize += uint64(l.idxSize)
	}

	l.items = make([]ilocItem, 0, count)
	for range count {
		item := ilocItem{id: c.readID(l.version == 2)}
		if l.version > 0 {
			item.method = uint8(c.read2() & 0xf)
		}
		item.dataRef = c.read2()
		item.base = c.readVarUint(l.baseSize)
		n := c.read2()
		if c.err != nil {
			return nil, c.err
		}
		if extentSize == 0 {
			if n > 1 {
				return nil, newInvalidFormatErrorf("avif: invalid iloc extent count %d", n)
			}
		} else if uint64(n) > uint64(c.remaining())/extentSize {
			return nil, newInvalidFormatErrorf("avif: invalid iloc extent count %d", n)
		}
		item.extents = make([]ilocExtent, 0, n)
		for range n {
			var ext ilocExtent
			if l.version > 0 {
				ext.index = c.readVarUint(l.idxSize)
			}
			ext.offset = c.readVarUint(l.offSize)
			ext.length = c.readVarUint(l.lenSize)
			if c.err != nil {
				return nil, c.err
			}
			item.extents = append(item.extents, ext)
		}
		l.items = append(l.items, item)
	}
	return l, c.err
}

func (l *iloc) encode() []byte {
	b := append([]byte{l.version}, l.flags[:]...)
	b = append(b, byte(l.offSize<<4|l.lenSize), byte(l.baseSize<<4|l.idxSize))
	b = appendID(b, l.version == 2, uint32(len(l.items)))
	for _, item := range l.items {
		b = appendID(b, l.version == 2, item.id)
		if l.version > 0 {
			b = binary.BigEndian.AppendUint16(b, uint16(item.method))
		}
		b = binary.BigEndian.AppendUint16(b, item.dataRef)
		b = appendUint(b, l.baseSize, item.base)
		b = binary.BigEndian.AppendUint16(b, uint16(len(item.extents)))
		for _, ext := range item.extents {
			if l.version > 0 {
				b = appendUint(b, l.idxSize, ext.index)
			}
			b = appendUint(b, l.offSize, ext.offset)
			b = appendUint(b, l.lenSize, ext.length)
		}
	}
	return b
}

// encode adds Exif and XMP items, stored in a new mdat box at the end of the file,
// linked to the primary item with cdsc references.
// Growing the meta box moves everything after it, so the iloc offsets are adjusted.
func (e *imageEncoderAVIF) encode() error {
	top, err := readISOBoxes(e.b, 0, len(e.b))
	if err != nil {
		return err
	}
	if len(top) == 0 || top[0].typ != heifFCC.ftyp {
		return newInvalidFormatErrorf("avif: missing ftyp box")
	}
	mi := -1
	for i, bx := range top {
		switch bx.typ {
		case heifFCC.meta:
			mi = i
		case heifFCC.moov:
			return errors.New("writing metadata to AVIF image sequences is not supported")
		}
	}
	if mi == -1 {
		return newInvalidFormatErrorf("avif: missing meta box")
	}
	meta := top[mi]
	if meta.end-meta.start < meta.hdr+4 {
		return newInvalidFormatErrorf("avif: invalid meta box")
	}
	children, err := readISOBoxes(e.b, meta.start+meta.hdr+4, meta.end)
	if err != nil {
		return err
	}

	var (
		primary          uint32
		loc              *iloc
		iinfIdx, irefIdx = -1, -1
		ilocIdx          = -1
		maxID            uint32
		iinfVersion      uint8
		iinfFlags        [3]byte
		infes            []byte
		infeCount        uint32
		irefVersion      uint8
		irefFlags        [3]byte
		irefChildren     []byte
	)
	for i, bx := range children {
		p := bx.payload(e.b)
		c := newSliceReader(p, binary.BigEndian)
		switch bx.typ {
		case heifFCC.pitm:
			primary = c.readID(c.read4()>>24 != 0)
		case heifFCC.iloc:
			ilocIdx = i
			if loc, err = parseIloc(p); err != nil {
				return err
			}
		case heifFCC.iinf:
			iinfIdx = i
			iinfVersion = c.read1()
			copy(iinfFlags[:], c.readBytes(3))
			infeCount = c.readID(iinfVersion != 0)
			if c.err != nil {
				return c.err
			}
			boxes, err := readISOBoxes(p, c.pos, len(p))
			if err != nil {
				return err
			}
			if infes, err = fixLastChildSizeToEOF(p[c.pos:], c.pos, boxes); err != nil {
				return err
			}
			for _, infe := range boxes {
				if infe.typ != heifFCC.infe {
					continue
				}
				ic := newSliceReader(infe.payload(p), binary.BigEndian)
				version := ic.read1()
				ic.skip(3)
				if version < 2 {
					id := uint32(ic.read2())
					if ic.err != nil {
						return ic.err
					}
					maxID = max(maxID, id)
					continue
				}
				id := ic.readID(version == 3)
				ic.skip(2) // item_protection_index
				itemType := ic.readFourCC()
				if ic.err != nil {
					return ic.err
				}
				maxID = max(maxID, id)
				if itemType == heifFCC.exif {
					return errMetadataExistsf("avif: %s item %d", itemType, id)
				}
				if itemType == heifFCC.mime {
					rest := ic.b[ic.pos:]
					if nul := bytes.IndexByte(rest, 0); nul >= 0 && bytes.HasPrefix(rest[nul+1:], heifXMPContentType) {
						return errMetadataExistsf("avif: %s item %d", itemType, id)
					}
				}
			}
		case heifFCC.iref:
			irefIdx = i
			irefVersion = c.read1()
			copy(irefFlags[:], c.readBytes(3))
			if c.err != nil {
				return c.err
			}
			boxes, err := readISOBoxes(p, 4, len(p))
			if err != nil {
				return err
			}
			if irefChildren, err = fixLastChildSizeToEOF(p[4:], 4, boxes); err != nil {
				return err
			}
		}
		if c.err != nil {
			return c.err
		}
	}
	if primary == 0 || loc == nil || iinfIdx == -1 {
		return newInvalidFormatErrorf("avif: missing pitm, iloc or iinf box")
	}
	maxID = max(maxID, primary)
	for _, item := range loc.items {
		maxID = max(maxID, item.id)
	}
	if loc.lenSize == 0 && len(loc.items) > 0 {
		return errors.New("avif: iloc length_size 0 is not supported")
	}

	type newItem struct {
		id       uint32
		typ      fourCC
		data     []byte
		mimeType []byte
	}
	var items []newItem
	if exif := e.meta.encodeEXIF(); exif != nil {
		items = append(items, newItem{typ: heifFCC.exif, data: append([]byte{0, 0, 0, 0}, exif...)})
	}
	items = append(items, newItem{typ: heifFCC.mime, data: e.meta.encodeXMP(), mimeType: heifXMPContentType})

	if maxID > math.MaxUint32-uint32(len(items)) {
		return errors.New("avif: too many items")
	}
	var wideIDs bool
	for i := range items {
		items[i].id = maxID + 1 + uint32(i)
		wideIDs = wideIDs || items[i].id > math.MaxUint16
	}
	if wideIDs && (loc.version < 2 || irefVersion == 0 && irefIdx != -1) {
		return errors.New("avif: too many items")
	}
	if iinfVersion == 0 && uint64(infeCount)+uint64(len(items)) > math.MaxUint16 {
		return errors.New("avif: too many items")
	}
	if loc.version < 2 && len(loc.items)+len(items) > math.MaxUint16 {
		return errors.New("avif: too many items")
	}
	if irefIdx == -1 && wideIDs {
		irefVersion = 1
	}

	// iinf.
	iinf := append([]byte{iinfVersion}, iinfFlags[:]...)
	iinf = appendID(iinf, iinfVersion != 0, infeCount+uint32(len(items)))
	iinf = append(iinf, infes...)
	for _, item := range items {
		infeVersion := byte(2)
		if item.id > math.MaxUint16 {
			infeVersion = 3
		}
		infe := appendID([]byte{infeVersion, 0, 0, 0}, infeVersion == 3, item.id)
		infe = append(infe, 0, 0) // item_protection_index
		infe = append(infe, item.typ[:]...)
		infe = append(infe, 0) // item_name
		infe = append(infe, item.mimeType...)
		iinf = appendISOBox(iinf, heifFCC.infe, infe)
	}

	// iref.
	iref := append(append([]byte{irefVersion}, irefFlags[:]...), irefChildren...)
	for _, item := range items {
		ref := appendID(nil, irefVersion != 0, item.id)
		ref = binary.BigEndian.AppendUint16(ref, 1)
		ref = appendID(ref, irefVersion != 0, primary)
		iref = appendISOBox(iref, heifFCC.cdsc, ref)
	}

	// iloc. Make room for 32-bit offsets and lengths; the encoded size does not depend on the values.
	loc.offSize, loc.lenSize = max(loc.offSize, 4), max(loc.lenSize, 4)
	oldItems := len(loc.items)
	for _, item := range items {
		loc.items = append(loc.items, ilocItem{id: item.id, extents: []ilocExtent{{length: uint64(len(item.data))}}})
	}

	encodeMeta := func() []byte {
		var bb []byte
		for i, bx := range children {
			switch i {
			case ilocIdx:
				bb = appendISOBox(bb, heifFCC.iloc, loc.encode())
			case iinfIdx:
				bb = appendISOBox(bb, heifFCC.iinf, iinf)
				if irefIdx == -1 {
					bb = appendISOBox(bb, heifFCC.iref, iref)
				}
			case irefIdx:
				bb = appendISOBox(bb, heifFCC.iref, iref)
			default:
				bb = append(bb, e.b[bx.start:bx.end]...)
			}
		}
		return appendISOBox(nil, heifFCC.meta, e.b[meta.start+meta.hdr:meta.start+meta.hdr+4], bb)
	}

	firstMeta := encodeMeta()
	if uint64(len(firstMeta)) > math.MaxUint32 {
		return errors.New("avif: meta box too large")
	}
	d := len(firstMeta) - (meta.end - meta.start)
	if d < 0 {
		return newInvalidFormatErrorf("avif: unexpected meta box size")
	}
	delta := uint64(d)
	for i := range loc.items[:oldItems] {
		item := &loc.items[i]
		if item.method != 0 || item.dataRef != 0 {
			continue
		}
		switch {
		case item.base >= uint64(meta.end):
			if item.base > math.MaxUint64-delta {
				return errors.New("avif: file too large")
			}
			item.base += delta
			if loc.baseSize == 4 && item.base > math.MaxUint32 {
				return errors.New("avif: file too large")
			}
		case item.base >= uint64(meta.start):
			return errors.New("avif: items stored in the meta box are not supported")
		default:
			for j := range item.extents {
				off := item.extents[j].offset
				if off > math.MaxUint64-item.base {
					return newInvalidFormatErrorf("avif: invalid iloc offset")
				}
				abs := item.base + off
				switch {
				case abs >= uint64(meta.end):
					if off > math.MaxUint64-delta {
						return errors.New("avif: file too large")
					}
					item.extents[j].offset = off + delta
					if loc.offSize == 4 && item.extents[j].offset > math.MaxUint32 {
						return errors.New("avif: file too large")
					}
				case abs >= uint64(meta.start):
					return errors.New("avif: items stored in the meta box are not supported")
				}
			}
		}
	}
	offset := uint64(len(e.b)) + delta + 8
	for i := range items {
		loc.items[oldItems+i].extents[0].offset = offset
		offset += uint64(len(items[i].data))
	}
	if loc.offSize == 4 && offset > math.MaxUint32 || loc.baseSize == 4 && uint64(len(e.b))+delta > math.MaxUint32 {
		return errors.New("avif: file too large")
	}

	newMeta := encodeMeta()
	var mdat [][]byte
	for _, item := range items {
		mdat = append(mdat, item.data)
	}
	mdatBox := appendISOBox(nil, heifFCC.mdat, mdat...)

	if last := top[len(top)-1]; last.sizeToEOF && mi != len(top)-1 {
		if last.hdr != 8 || last.end-last.start > math.MaxUint32 {
			return errors.New("avif: cannot fix size of last box")
		}
		var lastSize [4]byte
		binary.BigEndian.PutUint32(lastSize[:], uint32(last.end-last.start))
		return e.writeChunks(e.b[:meta.start], newMeta, e.b[meta.end:last.start], lastSize[:], e.b[last.start+4:], mdatBox)
	}
	return e.writeChunks(e.b[:meta.start], newMeta, e.b[meta.end:], mdatBox)
}
