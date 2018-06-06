// Package buildlog implements a logging system for build operations.
// Various parts of the API use Handler's to export logging information.
package buildlog

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Stream is a stream which Lines can be tagged with.
type Stream uint8

const (
	// StreamStdout is a LogStream for stdout.
	StreamStdout Stream = 1
	// StreamStderr is a LogStream for stderr.
	StreamStderr Stream = 2
	// StreamBuild is a LogStream for info from the build system.
	StreamBuild Stream = 3
	// StreamMeta is a LogStream for build metadata.
	StreamMeta Stream = 4
)

func (s Stream) String() string {
	switch s {
	case StreamStdout:
		return "stdout"
	case StreamStderr:
		return "stderr"
	case StreamBuild:
		return "build"
	case StreamMeta:
		return "meta"
	default:
		return "invalid"
	}
}

// Line is a line of log output.
type Line struct {
	Text   string `json:"text"`
	Stream Stream `json:"stream"`
}

func (ll Line) String() string {
	return fmt.Sprintf("[%s] %s", ll.Stream.String(), ll.Stream.String())
}

// Handler is an interface used for log output.
type Handler interface {
	Log(Line) error
	io.Closer
}

// goLogHandler is a LogHandler which uses a go builtin logger
type goLogHandler struct {
	l *log.Logger
}

func (glh *goLogHandler) Log(ll Line) error {
	glh.l.Println(ll.String())
	return nil
}

func (glh *goLogHandler) Close() error {
	return nil
}

// StdLogHandler creates a LogHandler which wraps a go stdlib logger.
// For this logger, Close is a no-op.
func StdLogHandler(l *log.Logger) Handler {
	return &goLogHandler{l}
}

// DefaultHandler is the default LogHandler.
// It logs to stderr.
var DefaultHandler = StdLogHandler(log.New(os.Stderr, "", log.LstdFlags))

// NewLogWriter returns an io.WriteCloser that is logged.
// The LogHandler must be mutexed if it is also used by anything else.
// Spawns a goroutine.
func NewLogWriter(lh Handler, stream Stream) io.WriteCloser {
	piper, pipew := io.Pipe()
	lw := &logWriter{
		pipew: pipew,
	}
	lw.doReadLog(lh, stream, piper)
	return lw
}

type logWriter struct {
	pipew *io.PipeWriter
	wg    sync.WaitGroup
	err   error
}

func (lw *logWriter) Write(data []byte) (int, error) {
	return lw.pipew.Write(data)
}

func (lw *logWriter) Close() error {
	err := lw.pipew.Close()
	if err != nil {
		return err
	}
	lw.wg.Wait()
	return lw.err
}

func (lw *logWriter) doReadLog(lh Handler, stream Stream, r io.Reader) {
	lw.wg.Add(1)
	go func() {
		defer lw.wg.Done()
		lw.err = ReadLog(lh, stream, r)
	}()
}

// ReadLog reads a log from a reader.
// The log is put to the Handler on the specified stream.
func ReadLog(lh Handler, stream Stream, r io.Reader) error {
	s := bufio.NewScanner(r)
	for s.Scan() {
		lh.Log(Line{
			Text:   s.Text(),
			Stream: stream,
		})
	}
	err := s.Err()
	if err != io.EOF {
		return err
	}
	return nil
}

// mutexedLogHandler is a Handler that uses a mutex to protect from concurrent access.
type mutexedLogHandler struct {
	lck sync.Mutex
	lh  Handler
}

func (mlh *mutexedLogHandler) Log(ll Line) error {
	mlh.lck.Lock()
	defer mlh.lck.Unlock()
	return mlh.lh.Log(ll)
}

func (mlh *mutexedLogHandler) Close() error {
	mlh.lck.Lock()
	defer mlh.lck.Unlock()
	return mlh.lh.Close()
}

// NewMutexedLogHandler returns a LogHandler which is thread-safe.
func NewMutexedLogHandler(handler Handler) Handler {
	return &mutexedLogHandler{lh: handler}
}

type metaInterceptor struct {
	cb func(string)
	lh Handler
}

func (mi *metaInterceptor) Log(ll Line) error {
	if ll.Stream == StreamMeta {
		mi.cb(ll.Text)
		return nil
	}
	return mi.lh.Log(ll)
}

func (mi *metaInterceptor) Close() error {
	return mi.lh.Close()
}

// InterceptMeta returrns a LogHandler which executes a callback instead of logging messages in StreamMeta.
func InterceptMeta(h Handler, callback func(string)) Handler {
	return &metaInterceptor{
		cb: callback,
		lh: h,
	}
}

type multiLogger []Handler

func (ml multiLogger) Log(ll Line) error {
	for _, v := range ml {
		err := v.Log(ll)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ml multiLogger) Close() error {
	for _, v := range ml {
		err := v.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// NewMultiLogHandler returns a LogHandler that logs to all given handlers.
func NewMultiLogHandler(handlers ...Handler) Handler {
	return multiLogger(handlers)
}
