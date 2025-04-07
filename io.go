// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"sync"
)

type bytesAndReader struct {
	b []byte
	r *bytes.Reader
}

var bytesAndReaderPool = &sync.Pool{
	New: func() any {
		return &bytesAndReader{
			b: make([]byte, 1024),
			r: bytes.NewReader(nil),
		}
	},
}

func getBytesAndReader(length int) *bytesAndReader {
	b := bytesAndReaderPool.Get().(*bytesAndReader)
	if length > cap(b.b) {
		b.b = make([]byte, length)
	}
	b.b = b.b[:length]
	return b
}

func putBytesAndReader(br *bytesAndReader) {
	br.b = br.b[:0]
	bytesAndReaderPool.Put(br)
}

var errShortRead = errors.New("short read")

func newStreamReader(r io.Reader, byteOrder binary.ByteOrder) *streamReader {
	return &streamReader{
		r:         r.(io.ReadSeeker),
		byteOrder: byteOrder,
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
	io.ReadSeeker
	io.Closer
}

// streamReader is a wrapper around a Reader that provides methods to read binary data.
// Note that this is not thread safe.
type streamReader struct {
	// The current Reader.
	r         io.ReadSeeker
	byteOrder binary.ByteOrder

	buf []byte

	isEOF        bool
	readErr      error
	readerOffset int64
}

var noopCloser closerFunc = func() error {
	return nil
}

func (e *streamReader) otherByteOrder() binary.ByteOrder {
	if e.byteOrder == binary.BigEndian {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

// 10 MB should be plenty for image metadata.
const maxBufSize = 10 * 1024 * 1024

// bufferedReader reads length bytes from the stream and returns a ReaderCloser.
// It's important to call Close on the ReaderCloser when done.
func (e *streamReader) bufferedReader(length int64) (readerCloser, error) {
	if length > maxBufSize {
		return nil, newInvalidFormatErrorf("length %d exceeds max %d", length, maxBufSize)
	}
	if length == 0 {
		return struct {
			io.ReadSeeker
			io.Closer
		}{
			bytes.NewReader(nil),
			noopCloser,
		}, nil
	}

	if length < 0 {
		return nil, newInvalidFormatErrorf("negative length")
	}

	br := getBytesAndReader(int(length))

	_, err := io.ReadFull(e.r, br.b)
	if err != nil {
		return nil, err
	}

	var closer closerFunc = func() error {
		putBytesAndReader(br)
		return nil
	}

	br.r.Reset(br.b)

	return struct {
		io.ReadSeeker
		io.Closer
	}{
		br.r,
		closer,
	}, nil
}

func (e *streamReader) allocateBuf(length int) {
	if length > cap(e.buf) {
		e.buf = make([]byte, length)
	}
}

func (e *streamReader) pos() int64 {
	n, _ := e.r.Seek(0, 1)
	return n
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

func (e *streamReader) read8r(r io.Reader) uint64 {
	const n = 8
	e.readNFromRIntoBuf(n, r)
	return e.byteOrder.Uint64(e.buf[:n])
}

func (e *streamReader) readBytes(b []byte) error {
	if _, err := io.ReadFull(e.r, b); err != nil {
		e.stop(err)
	}
	return nil
}

// readNullTerminatedBytes reads a slice of bytes from the stream
// until a null byte is encountered.
// It returns the slice of bytes read and the number of bytes read.
// Note that max is the maximum number of bytes to read including the null byte.
func (e *streamReader) readNullTerminatedBytes(max int) ([]byte, int64) {
	var b []byte
	var n int64
	for i := range max {
		b = append(b, e.read1())
		n++
		if b[i] == 0 {
			return b[:i], n
		}
	}
	return b, n
}

// readBytesVolatile reads a slice of bytes from the stream
// which is not guaranteed to be valid after the next read.
func (e *streamReader) readBytesVolatile(n int) []byte {
	e.readNIntoBuf(n)
	return e.buf[:n]
}

func (e *streamReader) readBytesVolatileE(n int) ([]byte, error) {
	err := e.readNIntoBufE(n)
	if err != nil {
		return nil, err
	}
	return e.buf[:n], nil
}

func (e *streamReader) readBytesFromRVolatile(n int, r io.Reader) []byte {
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

func (e *streamReader) preservePos(f func() error) error {
	pos := e.pos()
	err := f()
	e.seek(pos)
	return err
}

func (e *streamReader) seek(pos int64) {
	_, err := e.r.Seek(pos, io.SeekStart)
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
