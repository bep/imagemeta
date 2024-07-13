package imagemeta

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"sync"
)

var bufferPool = &sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

var errShortRead = errors.New("short read")

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

	s := &streamReader{
		r:         rr,
		byteOrder: binary.BigEndian,
	}

	return s
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
	// The current Reader.
	r Reader

	byteOrder binary.ByteOrder
	buf       []byte

	isEOF        bool
	readErr      error
	readerOffset int
}

func (e *streamReader) bufferedReader(length int) (readerCloser, error) {
	buff := getBuffer()
	n, err := io.CopyN(buff, e.r, int64(length))
	if err != nil {
		return nil, err
	}
	if n != int64(length) {
		return nil, errShortRead
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

func (e *streamReader) read2E() (uint16, error) {
	const n = 2
	if err := e.readNIntoBufE(n); err != nil {
		return 0, err
	}
	return e.byteOrder.Uint16(e.buf[:n]), nil
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

func (e *streamReader) readBytes(b []byte) error {
	if _, err := io.ReadFull(e.r, b); err != nil {
		e.stop(err)
	}
	return nil
}

// readBytesVolatile reads a slice of bytes from the stream
// which is not guaranteed to be valid after the next read.
func (e *streamReader) readBytesVolatile(n int) []byte {
	e.allocateBuf(n)
	e.readNIntoBuf(n)
	return e.buf[:n]
}

func (e *streamReader) readBytesVolatileE(n int) ([]byte, error) {
	e.allocateBuf(n)
	err := e.readNIntoBufE(n)
	if err != nil {
		return nil, err
	}
	return e.buf[:n], nil
}

func (e *streamReader) readBytesFromRVolatile(n int, r io.Reader) []byte {
	e.allocateBuf(n)
	e.readNFromRIntoBuf(n, r)
	return e.buf[:n]
}

func (e *streamReader) readNFromRIntoBuf(n int, r io.Reader) {
	if err := e.readNFromRIntoBufE(n, r); err != nil {
		e.stop(err)
	}
}

func (e *streamReader) readNFromRIntoBufE(n int, r io.Reader) error {
	e.allocateBuf(n)
	n2, err := io.ReadFull(r, e.buf[:n])
	if err != nil {
		return err
	}
	if n != n2 {
		return errShortRead
	}
	return nil
}

func (e *streamReader) readNIntoBuf(n int) {
	e.readNFromRIntoBuf(n, e.r)
}

func (e *streamReader) readNIntoBufE(n int) error {
	return e.readNFromRIntoBufE(n, e.r)
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
	// Alow one silent EOF.
	// This allows the client to not having to check for EOF on every read.
	if err == io.EOF && !e.isEOF {
		e.isEOF = true
		return
	}
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
