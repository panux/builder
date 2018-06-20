package sse

import (
	"bytes"
	"io"
	"testing"
)

func TestEventDecoding(t *testing.T) {
	cc := []struct {
		name  string
		input string
		event Event
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
			name:  "only a comment",
			input: ":start\n\n",
			err:   io.EOF,
		},
		{
			name:  "one data line",
			input: "data:ok\n\n",
			event: Event{Name: "message", Data: "ok"},
		},
		{
			name:  "one data line with leading space",
			input: "data: ok\n\n",
			event: Event{Name: "message", Data: "ok"},
		},
		{
			name:  "one data line with two leading spaces",
			input: "data:  ok\n\n",
			event: Event{Name: "message", Data: " ok"},
		},
		{
			name:  "comment at the beginning",
			input: ":some comment\ndata:ok\n\n",
			event: Event{Name: "message", Data: "ok"},
		},
		{
			name:  "comment at the end",
			input: "data:ok\n:some comment\n\n",
			event: Event{Name: "message", Data: "ok"},
		},
		{
			name:  "empty data",
			input: "data:\n\n",
			event: Event{Name: "message", Data: ""},
		},
		{
			name:  "empty data (without ':')",
			input: "data\n\n",
			event: Event{Name: "message", Data: ""},
		},
		{
			name:  "multiple data lines",
			input: "data:1\ndata: 2\ndata:3\n\n",
			event: Event{Name: "message", Data: "1\n2\n3"},
		},
		{
			name:  "typed event",
			input: "event:test\ndata:ok\n\n",
			event: Event{Name: "test", Data: "ok"},
		},
		{
			name:  "set id without data",
			input: "id:1\n\ndata:ok\n\n",
			event: Event{Name: "message", Data: "ok"},
		},
	}

	for _, c := range cc {
		t.Run(c.name, func(t *testing.T) {
			client := NewClient(bytes.NewBufferString(c.input))
			e, err := client.Event()
			if err != c.err {
				t.Errorf("got error '%v', expected '%v'", err, c.err)
			}
			if e != c.event {
				t.Errorf("got %#v, expected %#v", e, c.event)
			}
			err = client.Close()
			if err != nil {
				t.Errorf("could not close the client: %v", err)
			}
		})
	}
}

func TestSpecExamples(t *testing.T) {
	// taken from https://html.spec.whatwg.org/multipage/server-sent-events.html#dispatchMessage
	cc := []struct {
		name   string
		input  string
		events []Event
		err    error
	}{
		{
			name:   "stocks",
			input:  "data: YHOO\ndata: +2\ndata: 10\n\n",
			events: []Event{{Name: "message", Data: "YHOO\n+2\n10"}},
		},
		{
			name:  "four blocks",
			input: ": test stream\ndata: first event\nid: 1\n\ndata:second event\nid\n\ndata:  third event\n",
			events: []Event{
				{Name: "message", Data: "first event"},
				{Name: "message", Data: "second event"},
			},
		},
		{
			name:  "two event",
			input: "data\n\ndata\ndata\n\ndata:\n",
			events: []Event{
				{Name: "message", Data: ""},
				{Name: "message", Data: "\n"},
			},
		},
		{
			name:  "two identical events",
			input: "data:test\n\ndata: test\n\n",
			events: []Event{
				{Name: "message", Data: "test"},
				{Name: "message", Data: "test"},
			},
		},
	}

	for _, c := range cc {
		t.Run(c.name, func(t *testing.T) {
			client := NewClient(bytes.NewBufferString(c.input))
			for i, event := range c.events {
				e, err := client.Event()
				if err != c.err {
					t.Errorf("got error '%v', expected '%v' on event %d", err, c.err, i)
				}
				if e != event {
					t.Errorf("got %#v, expected %#v on event %d", e, event, i)
				}
			}
			e, err := client.Event()
			if err != io.EOF {
				t.Errorf("unexpected event at the end: %#v (error: %v)", e, err)
			}
			err = client.Close()
			if err != nil {
				t.Errorf("could not close the client: %v", err)
			}
		})
	}
}
