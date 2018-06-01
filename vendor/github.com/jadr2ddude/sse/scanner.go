package sse

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// Scanner parses an event stream
type Scanner struct {
	s *bufio.Scanner
}

// ScannedEvent is an SSE event.
// See https://html.spec.whatwg.org/multipage/server-sent-events.html#server-sent-events for more info.
type ScannedEvent struct {
	Type     string // event type
	Data     string // data buffer
	ID       string // last event ID
	IDSet    bool   // was the last event ID set
	Retry    int    // event stream's reconnection time
	RetrySet bool   // was the Retry delay set
}

// NewScanner returns a Scanner which will parse Event from the io.Reader
func NewScanner(r io.Reader) *Scanner {
	return &Scanner{s: bufio.NewScanner(r)}
}

// Event reads an event from the stream.
func (s *Scanner) Event() (ev ScannedEvent, err error) {
	for s.s.Scan() {
		line := s.s.Text()
		if line == "" {
			// dispatch the event
			return ev, nil
		}
		parts := strings.SplitN(line, ":", 2)
		key := parts[0]
		var val string
		if len(parts) > 1 {
			val = parts[1]
			if strings.HasPrefix(val, " ") {
				val = val[1:]
			}
		}

		switch key {
		case "event":
			ev.Type = val
		case "data":
			ev.Data += val + "\n"
		case "id":
			if !strings.ContainsRune(val, '\000') {
				ev.ID = val
				ev.IDSet = true
			}
		case "retry":
			retry, err := strconv.Atoi(val)
			if err == nil {
				ev.Retry = retry
				ev.RetrySet = true
			}
		default:
			// unsupported field
		}
	}
	err = s.s.Err()
	if err == nil {
		err = io.EOF
	}
	return ScannedEvent{}, err
}
