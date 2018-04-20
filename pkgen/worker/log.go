package worker

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

//LogStream is a stream which log lines can be tagged with
type LogStream uint8

const (
	//StreamStdout is a LogStream for stdout
	StreamStdout LogStream = 1
	//StreamStderr is a LogStream for stderr
	StreamStderr LogStream = 2
	//StreamBuild is a LogStream for info from the build system
	StreamBuild LogStream = 3
	//StreamMeta is a LogStream for metadata
	StreamMeta LogStream = 4
)

func (l LogStream) String() string {
	switch l {
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

//LogLine is a line of log output
type LogLine struct {
	Text   string    `json:"text"`
	Stream LogStream `json:"stream"`
}

func (ll LogLine) String() string {
	return fmt.Sprintf("[%s] %s", ll.Stream.String(), ll.Stream.String())
}

//LogHandler is an interface used for log output
type LogHandler interface {
	Log(LogLine) error
	io.Closer
}

//goLogHandler is a LogHandler which uses a go builtin logger
type goLogHandler struct {
	l *log.Logger
}

func (glh *goLogHandler) Log(ll LogLine) error {
	glh.l.Println(ll.String())
	return nil
}

func (glh *goLogHandler) Close() error {
	return nil
}

//StdLogHandler creates a LogHandler which wraps a go stdlib logger.
//For this logger, Close is a no-op.
func StdLogHandler(l *log.Logger) LogHandler {
	return &goLogHandler{l}
}

//DefaultLogHandler is the default LogHandler.
//It logs to stderr.
var DefaultLogHandler = StdLogHandler(log.New(os.Stderr, "", log.LstdFlags))

//NewLogWriter returns an io.WriteCloser that is logged.
//The LogHandler must be mutexed if it is also used by anything else.
//Spawns a goroutine.
func NewLogWriter(lh LogHandler, stream LogStream) io.WriteCloser {
	piper, pipew := io.Pipe()
	go ReadLog(lh, stream, piper)
	return pipew
}

//ReadLog reads a log from a reader.
//The log is put to the LogHandler on LogStream stream.
func ReadLog(lh LogHandler, stream LogStream, r io.Reader) error {
	s := bufio.NewScanner(r)
	for s.Scan() {
		lh.Log(LogLine{
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

type mutexedLogHandler struct {
	lck sync.Mutex
	lh  LogHandler
}

func (mlh *mutexedLogHandler) Log(ll LogLine) error {
	mlh.lck.Lock()
	defer mlh.lck.Unlock()
	return mlh.lh.Log(ll)
}

func (mlh *mutexedLogHandler) Close() error {
	mlh.lck.Lock()
	defer mlh.lck.Unlock()
	return mlh.lh.Close()
}

//NewMutexedLogHandler returns a LogHandler which is thread-safe.
func NewMutexedLogHandler(handler LogHandler) LogHandler {
	return &mutexedLogHandler{lh: handler}
}

type metaInterceptor struct {
	cb func(string)
	lh LogHandler
}

func (mi *metaInterceptor) Log(ll LogLine) error {
	if ll.Stream == StreamMeta {
		mi.cb(ll.Text)
		return nil
	}
	return mi.lh.Log(ll)
}

func (mi *metaInterceptor) Close() error {
	return mi.lh.Close()
}

//InterceptMeta returrns a LogHandler which executes a callback instead of logging messages in StreamMeta.
func InterceptMeta(lh LogHandler, callback func(string)) LogHandler {
	return &metaInterceptor{
		cb: callback,
		lh: lh,
	}
}
