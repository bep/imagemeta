package imagemeta

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
)

var bufferPool = &sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

func newStreamReader(r io.Reader) *streamReader {
	var rr Reader
	var ok bool
	rr, ok = r.(Reader)
	if !ok {
		bb, err := io.ReadAll(r)
		if err != nil {
			panic(err)
		}
		rr = bytes.NewReader(bb)
	}

	return &streamReader{
		r:         rr,
		byteOrder: binary.BigEndian,
	}
}

type decoder interface {
	decode() error
}

type streamReader struct {
	r Reader

	byteOrder binary.ByteOrder

	readErr      error
	readerOffset int
}

func (e *streamReader) bufferedReader(length int) (readerCloser, error) {
	buff := getBuffer()
	n, err := io.CopyN(buff, e.r, int64(length))
	if err != nil || n != int64(length) {
		return nil, err
	}
	r := bytes.NewReader(buff.Bytes())

	var closer closerFunc = func() error {
		putBuffer(buff)
		return nil
	}

	return struct {
		Reader
		io.Closer
	}{
		r,
		closer,
	}, nil

}

func (e *streamReader) pos() int {
	n, _ := e.r.Seek(0, 1)
	return int(n)
}

func (e *streamReader) read1() uint8 {
	return e.read1r(e.r)
}

func (e *streamReader) read1r(r io.Reader) uint8 {
	var v uint8
	e.readFullr(&v, r)
	return v
}

func (e *streamReader) read2() uint16 {
	return e.read2r(e.r)
}

func (e *streamReader) read2r(r io.Reader) uint16 {
	var v uint16
	e.readFullr(&v, r)
	return v
}

func (e *streamReader) read4() uint32 {
	return e.read4r(e.r)
}

func (e *streamReader) read4Signedr(r io.Reader) int32 {
	var v int32
	e.readFullr(&v, r)
	return v
}

func (e *streamReader) read4r(r io.Reader) uint32 {
	var v uint32
	e.readFullr(&v, r)
	return v
}

func (e *streamReader) readFull(v any) {
	e.readFullr(v, e.r)
}

func (e *streamReader) readFullE(v any) error {
	return e.readFullrE(v, e.r)
}

func (e *streamReader) readFullr(v any, r io.Reader) {
	if err := binary.Read(r, e.byteOrder, v); err != nil {
		e.stop(err)
	}
}

func (e *streamReader) readFullrE(v any, r io.Reader) error {
	return binary.Read(r, e.byteOrder, v)
}

func (e *streamReader) seek(pos int) {
	_, err := e.r.Seek(int64(pos), io.SeekStart)
	if err != nil {
		e.stop(err)
	}
}

func (e *streamReader) skip(n int64) {
	e.r.Seek(n, io.SeekCurrent)
}

func (e *streamReader) stop(err error) {
	if err != nil {
		e.readErr = err
	}
	panic(errStop)
}

func getBuffer() (buf *bytes.Buffer) {
	return bufferPool.Get().(*bytes.Buffer)
}

func putBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}
