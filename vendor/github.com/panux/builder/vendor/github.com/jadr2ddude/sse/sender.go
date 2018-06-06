//Package sse implements a server and client for the HTML5 Server-Sent-Events protocol.
//More protocol information available at https://html.spec.whatwg.org/multipage/server-sent-events.html#server-sent-events
package sse

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

//Sender is an HTML5 Server Sent Events sender
type Sender struct {
	hfl ResponseWriteFlusher
	buf *bufio.Writer
}

//NewSender creates an SSE event sender using an http.ResponseWriter
func NewSender(w http.ResponseWriter) (*Sender, error) {
	rf, err := Flusher(w)
	if err != nil {
		return nil, err
	}
	//set headers
	rf.Header().Set("Content-Type", "text/event-stream")
	rf.Header().Set("Cache-Control", "no-cache")
	rf.Header().Set("Connection", "keep-alive")
	rf.WriteHeader(http.StatusOK)
	return &Sender{hfl: rf, buf: bufio.NewWriter(rf)}, nil
}

//Event is an SSE event.
//See https://html.spec.whatwg.org/multipage/server-sent-events.html#server-sent-events for more info.
//The "id" and "retry" settings are not currently implemented.
type Event struct {
	Name string //Name of the event, referred to as "event" (optional)
	Data string //Event data, referred to as "data" (required)
}

//ErrIllegalRune is an error type which indicates that an illegal rune was in an Event field
type ErrIllegalRune struct {
	Name string //Name of field containing rune
	In   string //The string the rune is in
	Rune rune   //The illegal rune
}

func (err ErrIllegalRune) Error() string {
	return fmt.Sprintf("illegal rune in field %q: %q in %q", err.Name, err.Rune, err.In)
}

//ErrNoData is an error which indicates that an Event has an empty Data field
var ErrNoData = errors.New("event contains no data")

func validateString(str string, name string) error {
	illegals := "\n\r\000"
	if name == "data" {
		illegals = "\000"
	}
	if i := strings.IndexAny(str, illegals); i != -1 {
		return ErrIllegalRune{
			Name: name,
			In:   str,
			Rune: []rune(str)[i],
		}
	}
	return nil
}

//ErrNotNCName is an error indicating that the name of an Event is not a valid NCName
//See https://www.w3.org/TR/REC-xml-names/#NT-NCName
var ErrNotNCName = errors.New("name is not a valid NCName")

func checkNCName(name string) error {
	if strings.Contains(name, ":") {
		return ErrNotNCName
	}
	return nil
}

//Validate checks that an event is valid.
//An Event must have a Data field and fields cannot contain '\n' '\r' or '\0'.
//Data field is special and newlines are legal.
//Name must be a valid NCName (see ErrNotNCName).
func (ev Event) Validate() (err error) {
	if ev.Data == "" {
		return ErrNoData
	}
	err = validateString(ev.Name, "name")
	if err != nil {
		return
	}
	err = checkNCName(ev.Name)
	if err != nil {
		return
	}
	err = validateString(ev.Data, "data")
	if err != nil {
		return
	}
	return
}

func writeField(bw *bufio.Writer, name string, val string) int64 {
	if val == "" {
		return 0
	}
	if name == "data" && strings.ContainsAny(val, "\n\r") {
		var n int64
		for _, v := range strings.Split(strings.Replace(strings.Replace(val, "\r\n", "\n", -1), "\r", "\n", -1), "\n") {
			n += writeField(bw, name, v)
		}
		return n
	}
	n1, _ := bw.WriteString(name)
	n2, _ := bw.WriteString(": ")
	n3, _ := bw.WriteString(val)
	n4, _ := bw.WriteRune('\n')
	return int64(n1) + int64(n2) + int64(n3) + int64(n4)
}

//WriteTo allows the event to implement io.WriterTo
func (ev Event) WriteTo(w io.Writer) (int64, error) {
	err := ev.Validate()
	if err != nil {
		return 0, err
	}
	bw := bufio.NewWriter(w)
	var n int64
	n += writeField(bw, "event", ev.Name)
	n += writeField(bw, "data", ev.Data)
	n2, _ := bw.WriteRune('\n')
	return n + int64(n2), bw.Flush()
}

//SendEvent sends an event.
//The event is immediately flushed to the client.
func (s *Sender) SendEvent(event Event) error {
	err := s.SendQuick(event)
	if err != nil {
		return err
	}
	s.Flush()
	return nil
}

//SendQuick sends an event without flushing.
//When using this method, you must manually call Flush to writr the events.
//The purpose of this method is to speed up sending large batches of events.
func (s *Sender) SendQuick(event Event) error {
	_, err := event.WriteTo(s.buf)
	if err != nil {
		return err
	}
	return nil
}

//SendJSON sends a JSON event.
//The event is immediately flushed to the client.
func (s *Sender) SendJSON(msg interface{}) error {
	dat, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return s.SendEvent(Event{
		Data: string(dat),
	})
}

//Flush flushes the events.
//This is only necessary after SendQuick.
func (s *Sender) Flush() {
	s.hfl.Flush()
}

//ResponseWriteFlusher is an interface combining http.ResponseWriter and http.Flusher.
//Any http.ResponseWriter used for SSE must also implement http.Flusher.
type ResponseWriteFlusher interface {
	http.ResponseWriter
	http.Flusher
}

//ErrFlushNotSupported indicates that the provided http.ResponseWriter does not implement http.Flusher
var ErrFlushNotSupported = errors.New("flush not supported")

//Flusher tries to get a ResponseWriteFlusher from an http.ResponseWriter
func Flusher(w http.ResponseWriter) (ResponseWriteFlusher, error) {
	rf, ok := w.(ResponseWriteFlusher)
	if !ok {
		return nil, ErrFlushNotSupported
	}
	return rf, nil
}
