package bmapi

import (
	"encoding/gob"
	"fmt"
	"strings"

	"github.com/panux/builder/pkgen"
)

//Message is a message used in bmapi
type Message interface {
	m()
}

//ErrorMessage is a message containing an error
type ErrorMessage string

func (em ErrorMessage) m() {}
func (em ErrorMessage) String() string {
	return string(em)
}
func (em ErrorMessage) Error() string {
	return em.String()
}

//LogMessage is a messge output by the logger
type LogMessage struct {
	Text   string
	Stream uint8 //0 for stdin, 1 for stdout, 2 for stderr, 3 for builder
}

func (lm LogMessage) m() {}
func (lm LogMessage) String() string {
	return fmt.Sprintf("[%s] %s", strings.ToUpper(lm.StreamName()), lm.Text)
}

//StreamName returns the name of the stream
func (lm LogMessage) StreamName() string {
	switch lm.Stream {
	case 0:
		return "stdin"
	case 1:
		return "stdout"
	case 2:
		return "stderr"
	case 3:
		return "builder"
	default:
		return fmt.Sprintf("stream<%d>", lm.Stream)
	}
}

//DatMessage is a message used when sending a file
type DatMessage struct {
	Dat    []byte
	Stream uint32 //0 is hardcoded for build output on client
}

func (dm DatMessage) m() {}

//PkgenMessage is a message containing the PackageGenerator to build
type PkgenMessage struct {
	Gen *pkgen.PackageGenerator
}

func (pkm PkgenMessage) m() {}

//FileRequestMessage is a request to stream a file
type FileRequestMessage struct {
	Path   string
	Stream uint32
}

func (frm FileRequestMessage) m() {}

//PackageRequestMessage is a message requesting a package
type PackageRequestMessage struct {
	Name   string
	Stream uint32
}

func (prm PackageRequestMessage) m() {}

//StreamDoneMessage is a message sent to indicate that a stream has been closed
type StreamDoneMessage uint32

func (sdm StreamDoneMessage) m() {}

//DoneMessage is a message that everything is completed and has been sent
type DoneMessage struct{}

func (dm DoneMessage) m() {}

//PackagesReadyMessage is a message listing packages that are ready for transfer
//The client should respond with a PackageRequestMessage for each package
type PackagesReadyMessage []string

func (prm PackagesReadyMessage) m() {}

func init() {
	gob.RegisterName("bmapi.ErrorMessage", ErrorMessage(""))
	gob.RegisterName("bmapi.LogMessage", LogMessage{})
	gob.RegisterName("bmapi.DatMessage", DatMessage{})
	gob.RegisterName("bmapi.PkgenMessage", PkgenMessage{})
	gob.RegisterName("bmapi.FileRequestMessage", FileRequestMessage{})
	gob.RegisterName("bmapi.PackageRequestMessage", PackageRequestMessage{})
	gob.RegisterName("bmapi.StreamDoneMessage", StreamDoneMessage(0))
	gob.RegisterName("bmapi.DoneMessage", DoneMessage{})
}
