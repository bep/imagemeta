package imagemeta

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
)

// Decode reads EXIF and IPTC metadata from r and returns a Meta struct.
func Decode(opts Options) (Meta, error) {
	if opts.R == nil {
		return Meta{}, fmt.Errorf("need a reader")
	}

	return decode(opts)

}

// Meta contains the EXIF and IPTC metadata.
type Meta struct {
	EXIF Tags
	IPTC Tags
}

// DateTime returns the DateTime tag as a time.Time parsed with the given location.
func (m Meta) DateTime(loc *time.Location) time.Time {
	tags := m.EXIF
	if v, ok := tags["DateTime"]; ok {
		t, err := time.ParseInLocation("2006:01:02 15:04:05", v.(string), loc)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

// DateTimeOriginal returns the DateTimeOriginal tag as a time.Time parsed with the given location.
func (m Meta) DateTimeOriginal(loc *time.Location) time.Time {
	tags := m.EXIF
	if v, ok := tags["DateTimeOriginal"]; ok {
		t, err := time.ParseInLocation("2006:01:02 15:04:05", v.(string), loc)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

func (m Meta) LatLong() (float64, float64) {
	tags := m.EXIF
	longv := tags.get("GPSLongitude")
	ewv := tags.get("GPSLongitudeRef")
	latv := tags.get("GPSLatitude")
	nsv := tags.get("GPSLatitudeRef")

	fmt.Printf("longv: %v, ewv: %v, latv: %v, nsv: %v\n", longv, ewv, latv, nsv)

	if longv == nil || latv == nil {
		return 0, 0
	}

	long := tags.toDegrees(longv)
	lat := tags.toDegrees(latv)

	if ewv == "W" {
		long *= -1.0
	}
	if nsv == "S" {
		lat *= -1.0
	}

	return lat, long
}

// Orientation returns the Orientation tag.
func (m Meta) Orientation() Orientation {
	tags := m.EXIF
	if v, ok := tags["Orientation"]; ok {
		return Orientation(v.(uint16))
	}
	return OrientationUnspecified
}

type Options struct {
	// The Reader (typically a *os.File) to read EXIF data from.
	R Reader

	// If set, only these tags will be extracted.
	// This may speed up the decoding for large files if you are only interested in a few tags.
	// Note that this is case sensitive.
	// TODO1 implement
	TagSet map[string]bool

	// If set, the decoder will skip the EXIF data.
	SkipExif bool

	// If set, the decoder will skip the ITPC data.
	SkipITPC bool
}

type Orientation int

type Reader interface {
	io.ReadSeeker
	io.ReaderAt
}

// Tags is a map of EXIF and ITPC tags.
type Tags map[string]any

func (tags Tags) toDegrees(v any) float64 {
	if v == nil {
		return 0
	}
	f := [3]float64{}
	for i, vv := range v.([]interface{}) {
		if i >= 3 {
			break
		}
		r := vv.(*big.Rat)
		f[i], _ = r.Float64()
	}
	// TODO1 other types?
	return f[0] + f[1]/60 + f[2]/3600.0
}

func (tags Tags) get(key string) any {
	if v, ok := tags[key]; ok {
		return v
	}
	return nil
}

type decoder struct {
	r Reader

	readerOffset int
	byteOrder    binary.ByteOrder

	tagSet map[string]bool // May be nil.
	err    error

	tagsExif Tags
	tagsIPTC Tags
}

func (e *decoder) decode() (err error) {
	defer func() {
		if r := recover(); r != nil {
			if r != errStop {
				panic(r)
			}
			if err == nil {
				err = e.err
			}
		}
	}()

	// JPEG SOI marker.
	var soi uint16
	if err = e.readFullE(&soi); err != nil {
		return nil
	}
	if soi != markerSOI {
		return
	}

	findMarker := func(markerToFind uint16) int {
		for {
			var marker, length uint16
			if err = e.readFullE(&marker); err != nil {
				return -1
			}
			if err = e.readFullE(&length); err != nil {
				return -1
			}

			// All JPEG markers begin with 0xff.
			if marker>>8 != 0xff {
				return -1
			}

			if marker == markerToFind {
				return int(length)
			}

			if length < 2 {
				return -1
			}

			e.skip(int64(length - 2))
		}

	}

	pos := e.pos()
	if findMarker(markerAPP1) > 0 {
		if err := e.decodeExif(); err != nil {
			return err
		}
	}
	e.seek(pos)

	// EXIF may be stored in a different order, but IPTC is always big-endian.
	e.byteOrder = binary.BigEndian
	if length := findMarker(markerApp13); length > 0 {
		if err := e.decodeIPTC(length); err != nil {
			return err
		}
	}
	return nil
}

// A tag is represented in 12 bytes:
//   - 2 bytes for the tag ID
//   - 2 bytes for the data type
//   - 4 bytes for the number of data values of the specified type
//   - 4 bytes for the value itself, if it fits, otherwise for a pointer to another location where the data may be found;
//     this could be a pointer to the beginning of another IFD
func (e *decoder) decodeEXIFTag() error {
	tagID := e.read2()
	tagName := fieldsAll[tagID]
	if tagName == "" {
		tagName = fmt.Sprintf("%s0x%x", UnknownPrefix, tagID)
	}

	if !e.shouldDecode(tagName) {
		e.skip(10)
		return nil
	}

	dataType := e.read2()
	count := e.read4()
	if count > 0x10000 {
		e.skip(4)
		return nil
	}
	if count == 0 {
		count = 1
	}

	t := tagExif{
		id:    tagID,
		name:  tagName,
		typ:   exifType(dataType),
		count: count,
	}

	size, ok := typeSize[t.typ]
	if !ok {
		return fmt.Errorf("unknown type for tag %s %d", tagName, t.typ)
	}
	valLen := size * t.count

	var b []byte
	if valLen > 4 {
		offset := e.read4() + uint32(e.readerOffset)
		pos := e.pos()
		var buff bytes.Buffer
		sr := io.NewSectionReader(e.r, int64(offset), int64(valLen))
		n, err := io.Copy(&buff, sr)
		e.seek(pos)
		if err != nil || n != int64(valLen) {
			// TODO1
			return err
		}
		b = buff.Bytes()
	} else {
		b = make([]byte, valLen)
		if err := binary.Read(e.r, e.byteOrder, b); err != nil {
			return nil
		}
		padding := 4 - valLen
		if padding > 0 {
			e.skip(int64(padding))
		}
	}

	e.convertEXIFValues(&t, b)

	if strings.HasSuffix(tagName, "IFDPointer") {
		offset := t.sliceOrFirst().(uint32)
		return e.decodeEXIFTagsAt(int(offset))
	}

	e.tagsExif[t.name] = t.sliceOrFirst()

	return nil
}

func (e *decoder) done() bool {
	return e.tagSet != nil && len(e.tagSet) == 0
}

func (e *decoder) decodeEXIFTags() error {
	if e.done() {
		e.stop(nil)
	}

	numTags := e.read2()

	for i := 0; i < int(numTags); i++ {
		if err := e.decodeEXIFTag(); err != nil {
			return err
		}
	}

	return nil
}

func (e *decoder) decodeEXIFTagsAt(offset int) error {
	oldPos := e.pos()
	defer func() {
		e.seek(oldPos)
	}()
	e.seek(offset + e.readerOffset)
	return e.decodeEXIFTags()
}

func (e *decoder) decodeExif() (err error) {
	header := e.read4()

	if header != exifHeader {
		return nil
	}

	e.skip(2)

	byteOrderTag := e.read2()

	switch byteOrderTag {
	case byteOrderBigEndian:
		e.byteOrder = binary.BigEndian
	case byteOrderLittleEndian:
		e.byteOrder = binary.LittleEndian
	default:
		return nil
	}

	e.skip(2)

	// Skip the EXIF offset.
	offset := e.read4()

	if offset < 8 {
		return nil
	}

	e.skip(int64(offset - 8))
	e.readerOffset = e.pos() - 8

	return e.decodeEXIFTags()

}

func (e *decoder) convertEXIFValues(t *tagExif, b []byte) {
	r := bytes.NewReader(b)

	switch t.typ {
	case typeUnsignedByte, typeUndef:
		for i := 0; i < int(t.count); i++ {
			t.vals = append(t.vals, e.read1r(r))
		}
	case typeUnsignedAscii:
		t.vals = append(t.vals, string(b[:t.count-1]))
	case typeUnsignedShort:
		for i := 0; i < int(t.count); i++ {
			t.vals = append(t.vals, e.read2r(r))
		}
	case typeUnsignedLong:
		for i := 0; i < int(t.count); i++ {
			t.vals = append(t.vals, e.read4r(r))
		}
	case typeSignedLong:
		for i := 0; i < int(t.count); i++ {
			t.vals = append(t.vals, e.read4Signedr(r))
		}
	case typeUnsignedRat:
		for i := 0; i < int(t.count); i++ {
			n, d := e.read4r(r), e.read4r(r)
			t.vals = append(t.vals, big.NewRat(int64(n), int64(d)))
		}
	case typeSignedRat:
		for i := 0; i < int(t.count); i++ {
			n, d := e.read4Signedr(r), e.read4Signedr(r)
			t.vals = append(t.vals, big.NewRat(int64(n), int64(d)))
		}
	default:
		// TODO1
		panic(fmt.Errorf("exif type %d not implemented", t.typ))
	}
}

func (e *decoder) decodeIPTC(length int) (err error) {
	if e.done() {
		e.stop(nil)
	}

	// Skip the IPTC header.
	e.skip(14)

	const iptcMetaDataBlockID = 0x0404

	decodeBlock := func() error {
		blockType := make([]byte, 4)
		e.readFull(blockType)
		if string(blockType) != "8BIM" {
			return errStop
		}

		identifier := e.read2()
		isMeta := identifier == iptcMetaDataBlockID

		nameLength := e.read1()
		if nameLength == 0 {
			nameLength = 2
		} else if nameLength%2 == 1 {
			nameLength++
		}

		e.skip(int64(nameLength - 1))
		dataSize := e.read4()

		if !isMeta {
			e.skip(int64(dataSize))
			return nil
		}

		// TODO1 extended datasets.

		if dataSize%2 != 0 {
			defer func() {
				// Skip padding byte.
				e.skip(1)
			}()
		}

		r := io.LimitReader(e.r, int64(dataSize))

		for {
			var marker uint8
			if err := binary.Read(r, e.byteOrder, &marker); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			if marker != 0x1C {
				return errStop
			}

			var recordType, datasetNumber uint8
			var recordSize uint16
			if err := binary.Read(r, e.byteOrder, &recordType); err != nil {
				return err
			}
			if err := binary.Read(r, e.byteOrder, &datasetNumber); err != nil {
				return err
			}
			if err := binary.Read(r, e.byteOrder, &recordSize); err != nil {
				return err
			}

			recordBytes := make([]byte, recordSize)
			if err := binary.Read(r, e.byteOrder, recordBytes); err != nil {
				return err
			}

			// TODO1 get an up to date field map.
			// TODO1 handle unkonwn dataset numbers.
			recordDef, ok := iptcFieldMap[datasetNumber]
			if !ok {
				fmt.Println("unknown datasetNumber", datasetNumber)
				continue
			}
			if !e.shouldDecode(recordDef.name) {
				continue
			}
			var v any
			switch recordDef.format {
			case "string":
				v = string(recordBytes)
			case "B": // TODO1 check these
				v = recordBytes
			}

			e.tagsIPTC[recordDef.name] = v
		}
	}

	for {
		if err := decodeBlock(); err != nil {
			if err == errStop {
				break
			}
			return err
		}
	}

	return nil

}

func (e *decoder) pos() int {
	n, err := e.r.Seek(0, 1)
	if err != nil {
		e.stop(err)
	}
	return int(n)
}

func (e *decoder) read1() uint8 {
	return e.read1r(e.r)
}

func (e *decoder) read1r(r io.Reader) uint8 {
	var v uint8
	e.readFullr(&v, r)
	return v
}

func (e *decoder) read2() uint16 {
	return e.read2r(e.r)
}

func (e *decoder) read2r(r io.Reader) uint16 {
	var v uint16
	e.readFullr(&v, r)
	return v
}

func (e *decoder) read4() uint32 {
	return e.read4r(e.r)
}

func (e *decoder) read4Signedr(r io.Reader) int32 {
	var v int32
	e.readFullr(&v, r)
	return v
}

func (e *decoder) read4r(r io.Reader) uint32 {
	var v uint32
	e.readFullr(&v, r)
	return v
}

func (e *decoder) readFull(v any) {
	e.readFullr(v, e.r)
}

func (e *decoder) readFullE(v any) error {
	return e.readFullrE(v, e.r)
}

func (e *decoder) readFullr(v any, r io.Reader) {
	if err := binary.Read(r, e.byteOrder, v); err != nil {
		e.stop(err)
	}
}

func (e *decoder) readFullrE(v any, r io.Reader) error {
	return binary.Read(r, e.byteOrder, v)
}

func (e *decoder) seek(pos int) {
	_, err := e.r.Seek(int64(pos), io.SeekStart)
	if err != nil {
		e.stop(err)
	}
}

func (e *decoder) shouldDecode(tagName string) bool {
	if e.tagSet == nil {
		return true
	}
	b, found := e.tagSet[tagName]
	if found {
		delete(e.tagSet, tagName)
	}
	return b
}

func (e *decoder) skip(n int64) {
	_, err := e.r.Seek(n, io.SeekCurrent)
	if err != nil {
		e.stop(err)
	}
}

//lint:ignore U1000 // will be used later
func (e *decoder) skipr(n int64, r io.Reader) {
	switch r := r.(type) {
	case io.Seeker:
		r.Seek(n, io.SeekCurrent)
	default:
		io.CopyN(io.Discard, r, n)
	}
}

func (e *decoder) stop(err error) {
	if e.err != nil {
		e.err = err
	}
	panic(errStop)
}

// exifType represents the basic tiff tag data types.
type exifType uint16

type tagExif struct {
	id    uint16
	name  string
	typ   exifType
	count uint32
	vals  []any
}

func (t tagExif) sliceOrFirst() any {
	if len(t.vals) == 1 {
		return t.vals[0]
	}
	return t.vals
}

func decode(opts Options) (Meta, error) {
	m := Meta{
		EXIF: Tags{},
		IPTC: Tags{},
	}

	var tagSet map[string]bool
	if opts.TagSet != nil {
		tagSet = make(map[string]bool)
		for k, v := range opts.TagSet {
			tagSet[k] = v
		}
	}

	dec := &decoder{
		r:         opts.R,
		byteOrder: binary.BigEndian,
		tagSet:    tagSet,
		tagsExif:  m.EXIF,
		tagsIPTC:  m.IPTC,
	}

	if err := dec.decode(); err != nil {
		return m, err
	}

	return m, nil

}
