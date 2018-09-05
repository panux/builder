package buildlog

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

type nopCloseWriter struct {
	io.Writer
	closed bool
}

func (ncw *nopCloseWriter) Close() error {
	ncw.closed = true
	return nil
}

func TestJSONLog(t *testing.T) {
	logmsgs := []Line{
		Line{Stream: StreamStderr, Text: "stderr"},
		Line{Stream: StreamStdout, Text: "stdout"},
		Line{Stream: StreamBuild, Text: "build"},
		Line{Stream: StreamMeta, Text: "meta"},
	}

	// encode to JSON
	var buf bytes.Buffer
	ncw := &nopCloseWriter{Writer: &buf}
	jh, err := NewJSONHandler(ncw)
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
		return
	}
	for _, v := range logmsgs {
		err = jh.Log(v)
		if err != nil {
			t.Errorf("unexpected error: %s", err.Error())
			return
		}
	}
	err = jh.Close()
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
		return
	}
	if !ncw.closed {
		t.Error("io.WriteCloser not closed")
	}

	// decode JSON to log
	var out sliceHandler
	err = ReadJSONStream(&out, &buf)
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
		return
	}
	if !reflect.DeepEqual([]Line(out), logmsgs) {
		t.Errorf("expected %v but got %v", logmsgs, []Line(out))
	}
}
