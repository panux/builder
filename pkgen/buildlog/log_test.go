package buildlog

import (
	"reflect"
	"strings"
	"testing"
)

type sliceHandler []Line

func (sh *sliceHandler) Log(l Line) error {
	*sh = append(*sh, l)
	return nil
}

func (sh *sliceHandler) Close() error {
	return nil
}

func TestReadLog(t *testing.T) {
	tbl := []struct {
		in  string
		out []Line
	}{
		{
			in: "oneline",
			out: []Line{
				{Stream: StreamStderr, Text: "oneline"},
			},
		},
		{
			in: "two\nlines",
			out: []Line{
				{Stream: StreamStderr, Text: "two"},
				{Stream: StreamStderr, Text: "lines"},
			},
		},
		{
			in: "two\nlines\n",
			out: []Line{
				{Stream: StreamStderr, Text: "two"},
				{Stream: StreamStderr, Text: "lines"},
			},
		},
		{
			in: "three\n\nlines\n",
			out: []Line{
				{Stream: StreamStderr, Text: "three"},
				{Stream: StreamStderr, Text: ""},
				{Stream: StreamStderr, Text: "lines"},
			},
		},
	}
	for _, v := range tbl {
		var slh sliceHandler
		err := ReadLog(&slh, StreamStderr, strings.NewReader(v.in))
		if err != nil {
			t.Errorf("unexpected error %v", err.Error())
			continue
		}
		if !reflect.DeepEqual([]Line(slh), v.out) {
			t.Errorf("expected %v but got %v", v.out, []Line(slh))
			continue
		}
	}
}
