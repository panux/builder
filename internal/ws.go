package internal

import (
	"io"

	"github.com/gorilla/websocket"
)

type wsReader struct {
	c *websocket.Conn
	r io.Reader
}

func (wsr *wsReader) Read(dat []byte) (int, error) {
	if wsr.r == nil { //get another reader
		_, r, err := wsr.c.NextReader()
		if err != nil {
			return 0, err
		}
		wsr.r = r
	}
	n, err := wsr.r.Read(dat)
	if err == io.EOF { //finished reader mid-read
		wsr.r = nil
		n2, e2 := wsr.Read(dat[n:])
		return n + n2, e2
	}
	return n, err
}

//NewWebsocketReader returns an io.Reader that reads from a websocket.
//The io.Reader will read from multiple messages.
//Text and Binary messages are supported and can be mixed.
func NewWebsocketReader(ws *websocket.Conn) io.Reader {
	return &wsReader{c: ws}
}
