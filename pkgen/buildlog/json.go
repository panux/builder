package buildlog

import (
	"encoding/json"
	"fmt"
	"io"
)

type jsonHandler struct {
	w    io.WriteCloser
	je   *json.Encoder
	prev bool
}

var jsep = []byte{','}

func (jh *jsonHandler) Log(l Line) error {
	if !l.Stream.Valid() {
		return ErrInvalidStream
	}
	var err error
	if jh.prev {
		_, err = jh.w.Write(jsep)
		if err != nil {
			return err
		}
	} else {
		jh.prev = true
	}
	err = jh.je.Encode(l)
	if err != nil {
		return err
	}
	return nil
}

var obrk = []byte{'['}
var cbrk = []byte{']'}

func (jh *jsonHandler) Close() error {
	_, err := jh.w.Write(cbrk)
	cerr := jh.w.Close()
	if err == nil {
		err = cerr
	}
	return err
}

// NewJSONHandler returns a new Handler which writes a log as JSON to the given writer.
// If an error occured, the writer may or may not be closed.
func NewJSONHandler(w io.WriteCloser) (Handler, error) {
	_, err := w.Write(obrk)
	if err != nil {
		return nil, err
	}
	return &jsonHandler{
		w:  w,
		je: json.NewEncoder(w),
	}, nil
}

// ReadJSONStream reads a log from a JSON array.
func ReadJSONStream(dst Handler, src io.Reader) error {
	jd := json.NewDecoder(src)

	// read open bracket
	tok, err := jd.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('[') {
		return fmt.Errorf("unexpected token %v", tok)
	}

	// read lines
	var l Line
	for jd.More() {
		err = jd.Decode(&l)
		if err != nil {
			return err
		}
		err = dst.Log(l)
		if err != nil {
			return err
		}
	}

	// read closing bracket
	tok, err = jd.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim(']') {
		return fmt.Errorf("unexpected token %v", tok)
	}

	return nil
}
