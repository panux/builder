package buildlog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Logger is a generic logging service.
type Logger interface {
	// NewLog returns a Handler to write a new log.
	// The name be valid according to ValidateName.
	NewLog(name string) (Handler, error)
}

// LogReader is a generic log-reading interface.
type LogReader interface {
	// ReadLog reads a log into the Handler.
	// The name be valid according to ValidateName.
	ReadLog(name string, h Handler) error
}

// LogStore is an interface for saving and reading logs.
type LogStore interface {
	Logger
	LogReader
}

// IllegalRuneError is an error type for when an invalid rune is found.
type IllegalRuneError struct {
	// Rune is the illegal rune.
	Rune rune

	// Pos is the index of the rune (by rune - not by byte).
	Pos int

	// Strign is the original string containing the invalid rune.
	String string
}

func (e IllegalRuneError) Error() string {
	return fmt.Sprintf("illegal rune %q at %d in %q", e.Rune, e.Pos, e.String)
}

// ValidateName validates a name which must only contain [a-z] or [_- ].
func ValidateName(name string) error {
	for i, r := range name {
		// check for alphanumeric
		if ('a' <= r && r <= 'z') || ('0' <= r && r <= '9') {
			continue
		}

		// other legal runes
		switch r {
		case '_', '-', ' ':
			continue
		}

		// handle illegal rune
		return IllegalRuneError{
			Rune:   r,
			Pos:    i,
			String: name,
		}
	}

	return nil
}

// DirLogger is a LogStore implementation which stores JSON logs in a directory.
type DirLogger struct {
	Dir string
}

// NewLog returns a handler for a new log, implementing the Logger interface.
func (d DirLogger) NewLog(name string) (Handler, error) {
	// validate name
	err := ValidateName(name)
	if err != nil {
		return nil, err
	}

	// open log file
	f, err := os.OpenFile(filepath.Join(d.Dir, name), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	// create JSON streamer
	h, err := NewJSONHandler(f)
	if err != nil {
		f.Close()
		return nil, err
	}

	return h, nil
}

// ReadLog reads a log into the Handler.
func (d DirLogger) ReadLog(name string, h Handler) (err error) {
	// validate name
	err = ValidateName(name)
	if err != nil {
		return err
	}

	// open log file
	f, err := os.Open(filepath.Join(d.Dir, name))
	if err != nil {
		return err
	}
	defer func() {
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
	}()

	// decode JSON stream
	return ReadJSONStream(h, f)
}

type textLogger struct {
	w   io.Writer
	lck sync.Mutex
}

func (tl *textLogger) logStream(name string, line Line) error {
	tl.lck.Lock()
	defer tl.lck.Unlock()

	_, err := fmt.Fprintf(tl.w, "[%s][%s] %s\n", name, line.Stream.String(), line.Text)
	if err != nil {
		return err
	}

	return nil
}

func (tl *textLogger) NewLog(name string) (Handler, error) {
	return &textLoggerHandler{
		name: name,
		tl:   tl,
	}, nil
}

type textLoggerHandler struct {
	name string
	tl   *textLogger
}

func (th *textLoggerHandler) Log(line Line) error {
	return th.tl.logStream(th.name, line)
}

func (th *textLoggerHandler) Close() error {
	return nil
}

// TextLogger returns a Logger that logs to the given io.Writer in human-readable format.
func TextLogger(w io.Writer) Logger {
	return &textLogger{w: w}
}
