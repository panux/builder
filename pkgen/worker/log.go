package worker

import (
	"fmt"
	"io"
	"log"
	"os"
)

//LogStream is a stream which log lines can be tagged with
type LogStream uint8

const (
	//StreamStdout is a LogStream for stdout
	StreamStdout LogStream = 1
	//StreamStderr is a LogStream for stderr
	StreamStderr LogStream = 2
	//StreamMeta is a LogStream for the server executing the command/build
	StreamMeta LogStream = 3
)

func (l LogStream) String() string {
	switch l {
	case StreamStdout:
		return "stdout"
	case StreamStderr:
		return "stderr"
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
