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

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}

type decoder interface {
	decode() error
}

type fourCC [4]byte

type readerCloser interface {
	Reader
	io.Closer
}

// streamReader is a wrapper around a Reader that provides methods to read binary data.
// Note that this is not thread safe.
type streamReader struct {
	r Reader

	byteOrder binary.ByteOrder
	buf       []byte

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

func (e *streamReader) allocateBuf(length int) {
	if length > len(e.buf) {
		e.buf = make([]byte, length)
	}
}

func (e *streamReader) pos() int {
	n, _ := e.r.Seek(0, 1)
	return int(n)
}

func (e *streamReader) read1() uint8 {
	return e.read1r(e.r)
}

func (e *streamReader) read1r(r io.Reader) uint8 {
	const n = 1
	e.readNFromRIntoBuf(n, r)
	return e.buf[0]
}

func (e *streamReader) read2() uint16 {
	return e.read2r(e.r)
}

func (e *streamReader) read2r(r io.Reader) uint16 {
	const n = 2
	e.readNFromRIntoBuf(n, r)
	return e.byteOrder.Uint16(e.buf[:n])
}

func (e *streamReader) read4() uint32 {
	const n = 4
	e.readNIntoBuf(n)
	return e.byteOrder.Uint32(e.buf[:n])
}

func (e *streamReader) read4r(r io.Reader) uint32 {
	const n = 4
	e.readNFromRIntoBuf(n, r)
	return e.byteOrder.Uint32(e.buf[:n])
}

func (e *streamReader) read4sr(r io.Reader) int32 {
	const n = 4
	e.readNFromRIntoBuf(n, r)
	return int32(e.byteOrder.Uint32(e.buf[:n]))
}

func (e *streamReader) readBytes(b []byte) {
	if _, err := io.ReadFull(e.r, b); err != nil {
		e.stop(err)
	}
}

// readBytesVolatile reads a slice of bytes from the stream
// which is not guaranteed to be valid after the next read.
func (e *streamReader) readBytesVolatile(n int) []byte {
	e.allocateBuf(n)
	e.readNIntoBuf(n)
	return e.buf[:n]
}

func (e *streamReader) readFullE(v any) error {
	return e.readFullrE(v, e.r)
}

func (e *streamReader) readFullrE(v any, r io.Reader) error {
	return binary.Read(r, e.byteOrder, v)
}

func (e *streamReader) readNFromRIntoBuf(n int, r io.Reader) {
	e.allocateBuf(n)
	if _, err := io.ReadFull(r, e.buf[:n]); err != nil {
		e.stop(err)
	}
}

func (e *streamReader) readNIntoBuf(n int) {
	e.readNFromRIntoBuf(n, e.r)
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
