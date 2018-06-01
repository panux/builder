package sse

import (
	"errors"
	"io"
	"net/http"
)

//Client is a SSE client
type Client struct {
	s           *Scanner
	lastEventID string
	close       func() error
}

// NewClient returns a client which will parse Event from the io.Reader
func NewClient(r io.Reader) *Client {
	s := NewScanner(r)
	var close func() error
	closer, ok := (r).(io.Closer)
	if ok {
		close = closer.Close
	}
	return &Client{s: s, close: close}
}

//ErrClosedClient is an error indicating that the client has been used after it was closed.
var ErrClosedClient = errors.New("client closed")

// Event reads an event from the stream.
func (c *Client) Event() (ev Event, err error) {
	if c.s == nil {
		return Event{}, ErrClosedClient
	}

	// According to https://html.spec.whatwg.org/multipage/server-sent-events.html#dispatchMessage
	var event ScannedEvent

	// "2. If the data buffer is an empty string (...) return" (ie: don't dispatch an event)
	for event.Data == "" {
		event, err = c.s.Event()
		// todo: set the event stream's reconnection time
		if err != nil {
			return Event{}, err
		}
		// "1. Set the last event ID string of the event source"
		if event.IDSet {
			c.lastEventID = event.ID
		}
	}
	if event.Type == "" {
		// 5. Initialize event's type attribute to message
		ev.Name = "message"
	} else {
		// 6. If the event type buffer has a value other than the empty string
		ev.Name = event.Type
	}

	// 3. If the data buffer's last character is a \n (...) remove [it]
	ev.Data = event.Data[:len(event.Data)-1]

	// todo: set lastEventID on event (5.)

	return ev, nil
}

//Close closes the client
func (c *Client) Close() error {
	c.s = nil
	if c.close != nil {
		return c.close()
	}
	return nil
}

//ErrNotSSE is an error returned when a client receives a non-SSE response
var ErrNotSSE = errors.New("content type is not 'text/event-stream'")

//Connect performs an SSE request and returns a Client.
func Connect(client *http.Client, request *http.Request) (*Client, error) {
	request.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		resp.Body.Close()
		return nil, ErrNotSSE
	}
	return NewClient(resp.Body), nil
}
