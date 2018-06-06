package sse

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidate(t *testing.T) {
	testtable := []struct {
		ev  Event
		err error
	}{
		{Event{}, ErrNoData},
		{Event{Data: "wow\nmultiline"}, nil},
		{Event{Data: "wow\rmultiline"}, nil},
		{Event{Data: "\000"}, ErrIllegalRune{Rune: '\000', In: "\000", Name: "data"}},
		{Event{Name: "\000", Data: "test"}, ErrIllegalRune{Rune: '\000', In: "\000", Name: "name"}},
		{Event{Name: "\n", Data: "test"}, ErrIllegalRune{Rune: '\n', In: "\n", Name: "name"}},
		{Event{Name: "\r", Data: "test"}, ErrIllegalRune{Rune: '\r', In: "\r", Name: "name"}},
		{Event{Name: "test:testing", Data: "good"}, ErrNotNCName},
	}
	for _, tv := range testtable {
		err := tv.ev.Validate()
		if err != tv.err {
			t.Errorf("Expected error %q but got %q", tv.err, tv.err)
		}
	}
}

func TestSendEvent(t *testing.T) {
	testtable := []struct {
		ev     Event
		output string
		err    error
	}{
		{Event{Data: "testing123"}, "data: testing123\n\n", nil},
		{Event{Name: "test", Data: "testing123"}, "event: test\ndata: testing123\n\n", nil},
		{Event{Name: "testmultiline1", Data: "testing123\n\rtesting456"}, "event: testmultiline1\ndata: testing123\ndata: testing456\n\n", nil},
		{Event{Name: "testmultiline2", Data: "testing123\ntesting456"}, "event: testmultiline2\ndata: testing123\ndata: testing456\n\n", nil},
		{Event{Name: "testmultiline3", Data: "testing123\rtesting456"}, "event: testmultiline3\ndata: testing123\ndata: testing456\n\n", nil},
		{Event{}, "", ErrNoData},
	}
	for _, tv := range testtable {
		rr := httptest.NewRecorder()
		s, err := NewSender(rr)
		if err != nil {
			t.Errorf("failed to create sender: %q", err)
			continue
		}
		err = s.SendEvent(tv.ev)
		if err != tv.err {
			t.Errorf("expected error %q but got %q", tv.err, err)
			continue
		}
		if err == nil && !rr.Flushed {
			t.Errorf("Did not flush (on test case %v)", tv)
		}
		output, _ := ioutil.ReadAll(rr.Result().Body)
		if !bytes.Equal(output, []byte(tv.output)) {
			t.Errorf("Expected %q but got %q", tv.output, string(output))
		}
	}
}

func TestSendJSON(t *testing.T) {
	testtable := []struct {
		in     interface{}
		output string
		err    error
	}{
		{true, "data: true\n\n", nil},
		{[]int{1, 2, 3}, "data: [1,2,3]\n\n", nil},
		{nil, "data: null\n\n", nil},
	}
	for _, tv := range testtable {
		rr := httptest.NewRecorder()
		s, err := NewSender(rr)
		if err != nil {
			t.Errorf("failed to create sender: %q", err)
			continue
		}
		err = s.SendJSON(tv.in)
		if err != tv.err {
			t.Errorf("expected error %q but got %q", tv.err, err)
			continue
		}
		output, _ := ioutil.ReadAll(rr.Result().Body)
		if !bytes.Equal(output, []byte(tv.output)) {
			t.Errorf("Expected %q but got %q", tv.output, string(output))
		}
	}
}

type badWriter struct{}

func (badWriter) Header() http.Header        { return nil }
func (badWriter) Write([]byte) (int, error)  { return 0, nil }
func (badWriter) WriteHeader(statusCode int) {}

func TestFlusher(t *testing.T) {
	testtbl := []struct {
		w   http.ResponseWriter
		err error
	}{
		{httptest.NewRecorder(), nil},
		{badWriter{}, ErrFlushNotSupported},
	}
	for _, tv := range testtbl {
		rwf, err := Flusher(tv.w)
		if err != tv.err {
			t.Errorf("expected error %q but got %q", tv.err, err)
			continue
		}
		if err == nil && rwf == nil {
			t.Errorf("no error but no flusher returned (test case %v)", tv)
			continue
		}
	}
}
