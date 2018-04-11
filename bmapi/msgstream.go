package bmapi

import (
	"encoding/gob"
	"errors"
	"io"
	"sync"
)

//MsgStreamWriter is a writer for Message streams
type MsgStreamWriter struct {
	enc *gob.Encoder
	lck sync.Mutex
}

//Send sends a message over the writer (thread-safe)
func (msw *MsgStreamWriter) Send(m Message) error {
	msw.lck.Lock()
	defer msw.lck.Unlock()
	return msw.enc.Encode(m)
}

type datWriter struct {
	msw     *MsgStreamWriter
	streamn uint32
}

func (dw *datWriter) Write(dat []byte) (int, error) {
	if dw.msw == nil {
		return 0, io.ErrClosedPipe
	}
	err := dw.msw.Send(DatMessage{
		Stream: dw.streamn,
		Dat:    dat,
	})
	return 0, err
}
func (dw *datWriter) Close() error {
	if dw.msw == nil {
		return io.ErrClosedPipe
	}
	err := dw.msw.Send(StreamDoneMessage(dw.streamn))
	dw.msw = nil
	return err
}

//Stream returns an io.WriteCloser which writes to a stream
func (msw *MsgStreamWriter) Stream(streamnum uint32) io.WriteCloser {
	return &datWriter{
		streamn: streamnum,
		msw:     msw,
	}
}

//NewMsgStreamWriter returns a new MsgStreamWriter which writes Messages to w
func NewMsgStreamWriter(w io.Writer) *MsgStreamWriter {
	return &MsgStreamWriter{
		enc: gob.NewEncoder(w),
	}
}

//MsgStreamReader is a reader for Messages
type MsgStreamReader struct {
	dec     *gob.Decoder
	dstrs   map[uint32]chan<- []byte
	streamn uint32
}

//NextMessage reads the next message from the MsgStreamReader
func (msr *MsgStreamReader) NextMessage() (Message, error) {
	var m Message
	err := msr.dec.Decode(&m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

//ErrStreamNotFound is returned when a stream is not connected
var ErrStreamNotFound = errors.New("stream not found")

//ErrBadMessage is returned when a Message cannot be processed
var ErrBadMessage = errors.New("bad message")

//HandleDat handles a data message (DatMessage or StreamDoneMessage)
func (msr *MsgStreamReader) HandleDat(m Message) error {
	switch msg := m.(type) {
	case DatMessage:
		dch := msr.dstrs[msg.Stream]
		if dch == nil {
			return ErrStreamNotFound
		}
		dch <- msg.Dat
	case StreamDoneMessage:
		dch := msr.dstrs[uint32(msg)]
		if dch == nil {
			return ErrStreamNotFound
		}
		close(dch)
	default:
		return ErrBadMessage
	}
	return nil
}

type datReader struct {
	bch <-chan []byte
	buf []byte
}

func (dr *datReader) Read(buf []byte) (int, error) {
	if len(dr.buf) > 0 {
		n := copy(buf, dr.buf)
		dr.buf = dr.buf[n:]
		return n, nil
	}
	dat, ok := <-dr.bch
	if !ok {
		return 0, io.EOF
	}
	dr.buf = dat
	return dr.Read(buf)
}

//Stream returns a stream number and an io.Reader which will read data from the stream
//The io.Reader should be read from another goroutine
func (msr *MsgStreamReader) Stream(buf int) (uint32, io.Reader) {
	defer func() {
		msr.streamn++
	}()
	strch := make(chan []byte, buf)
	msr.dstrs[msr.streamn] = strch
	return msr.streamn, &datReader{bch: strch}
}

//Halt closes all stream readers immediately
//The MsgStreamReader should not be used after this call
//Why: for error handling
//NOTE: data may be lost
func (msr *MsgStreamReader) Halt() {
	for _, v := range msr.dstrs {
		close(v)
	}
}

//NewMsgStreamReader returns a new MsgStreamReader
func NewMsgStreamReader(r io.Reader) *MsgStreamReader {
	return &MsgStreamReader{
		dec:   gob.NewDecoder(r),
		dstrs: make(map[uint32]chan<- []byte),
	}
}
