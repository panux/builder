package sse

import (
	"bytes"
	"io"
	"testing"
)

func TestScannedEventDecoding(t *testing.T) {
	cc := []struct {
		name  string
		input string
		event ScannedEvent
		err   error
	}{
		{
			name:  "empty_event",
			input: "",
			err:   io.EOF,
		},
		{
			name:  "incomplete_event",
			input: "data:ok\n",
			err:   io.EOF,
		},
		{
			name:  "one data line",
			input: "data:ok\n\n",
			event: ScannedEvent{Data: "ok\n"},
		},
		{
			name:  "one data line with retry",
			input: "data:ok\nretry:15\n\n",
			event: ScannedEvent{Data: "ok\n", Retry: 15, RetrySet: true},
		},
		{
			name:  "one data line with zero retry",
			input: "data:ok\nretry: 0\n\n",
			event: ScannedEvent{Data: "ok\n", RetrySet: true},
		},
		{
			name:  "one data line with type",
			input: "event: type\ndata:ok\n\n",
			event: ScannedEvent{Type: "type", Data: "ok\n"},
		},
		{
			name:  "one data line with id",
			input: "data:ok\nid:1\n\n",
			event: ScannedEvent{ID: "1", IDSet: true, Data: "ok\n"},
		},
		{
			name:  "one data line with empty id",
			input: "data:ok\nid\n\n",
			event: ScannedEvent{IDSet: true, Data: "ok\n"},
		},
		{
			name:  "U+0000 in ID",
			input: "id:\0001\n\n",
			event: ScannedEvent{},
		},
		{
			name:  "one data line with leading space",
			input: "data: ok\n\n",
			event: ScannedEvent{Data: "ok\n"},
		},
		{
			name:  "one data line with two leading spaces",
			input: "data:  ok\n\n",
			event: ScannedEvent{Data: " ok\n"},
		},
		{
			name:  "comment at the beginning",
			input: ":some comment\ndata:ok\n\n",
			event: ScannedEvent{Data: "ok\n"},
		},
		{
			name:  "comment at the end",
			input: "data:ok\n:some comment\n\n",
			event: ScannedEvent{Data: "ok\n"},
		},
		{
			name:  "empty data",
			input: "data:\n\n",
			event: ScannedEvent{Data: "\n"},
		},
		{
			name:  "empty data (without ':')",
			input: "data\n\n",
			event: ScannedEvent{Data: "\n"},
		},
		{
			name:  "multiple data lines",
			input: "data:1\ndata: 2\ndata:3\n\n",
			event: ScannedEvent{Data: "1\n2\n3\n"},
		},
		{
			name:  "data with colon",
			input: "data: key:value\n\n",
			event: ScannedEvent{Data: "key:value\n"},
		},
	}

	for _, c := range cc {
		t.Run(c.name, func(t *testing.T) {
			scanner := NewScanner(bytes.NewBufferString(c.input))
			e, err := scanner.Event()
			if err != c.err {
				t.Errorf("got error '%v', expected '%v'", err, c.err)
			}
			if e != c.event {
				t.Errorf("got %#v, expected %#v", e, c.event)
			}
		})
	}
}
