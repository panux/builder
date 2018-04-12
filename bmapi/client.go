package bmapi

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/panux/builder/pkgen"
	"golang.org/x/net/websocket"
	"golang.org/x/tools/godoc/vfs"
)

//Client is a BuildManager client
type Client struct {
	turl *url.URL
	hcl  *http.Client
	dial *net.Dialer
}

//NewClient creates a new Client
func NewClient(u *url.URL, nd *net.Dialer, hcl *http.Client) *Client {
	return &Client{
		turl: u,
		dial: nd,
		hcl:  hcl,
	}
}

//Status is the JSON struct sent by /status
type Status struct {
	State   string `json:"state"` //should be "running"
	Version string `json:"version"`
}

//Status sends a request to /status and returns the JSON
func (cli *Client) Status() (*Status, error) {
	ru, err := url.Parse("/status")
	if err != nil {
		return nil, err
	}
	ru = cli.turl.ResolveReference(ru)
	g, err := cli.hcl.Get(ru.String())
	if err != nil {
		return nil, err
	}
	defer g.Body.Close()
	st := new(Status)
	err = json.NewDecoder(g.Body).Decode(st)
	if err != nil {
		return nil, err
	}
	return st, nil
}

//PackageWriteHandler is a function that saves a package in the io.ReadCloser with the name specified by the string.
//Package is in .tar.gz format.
type PackageWriteHandler func(string, io.ReadCloser) error

//PackageGetter is a function that loads a package with the specified name into the io.WriteCloser.
//Package is in .tar.gz format.
type PackageGetter func(string, io.WriteCloser) error

//BuildSettings is a struct containing the settings for a Build op
type BuildSettings struct {
	//Log is where log messages get sent. Closed after use.
	//Optional (defaults to go log).
	Log chan<- LogMessage
	//The vfs to load files from
	//Required.
	FS vfs.FileSystem
	//The PackageWriter to use
	//Required.
	PackageWriter PackageWriteHandler
	//The PackageGetter to use
	//Required.
	PackageGetter PackageGetter
}

//Build runs a build using the BuildManager.
func (cli *Client) Build(pk *pkgen.PackageGenerator, bs BuildSettings) error {
	fs, gpkg, logch, wpkg := bs.FS, bs.PackageGetter, bs.Log, bs.PackageWriter
	if logch == nil {
		lch := make(chan LogMessage)
		logch = lch
		go func() {
			for m := range lch {
				log.Println(m)
			}
		}()
	}
	//genetate request URL
	ru, err := url.Parse("/build")
	if err != nil {
		return err
	}
	ru = cli.turl.ResolveReference(ru)
	//Connect to server
	ws, err := websocket.DialConfig(&websocket.Config{
		Version:  websocket.ProtocolVersionHybi13,
		Location: ru,
		Dialer:   cli.dial,
		Header:   http.Header(make(map[string][]string)),
	})
	if err != nil {
		return err
	}
	//Run processing
	return WorkMsgConn(ws, func(w *MsgStreamWriter, r *MsgStreamReader) error {
		//create channel for sending fatal errors
		errch := make(chan error)
		defer close(errch)
		//create handler for file requests
		flh := NewFileLoadHandler(w, fs)
		droperr := func() {
			err := recover()
			if err != nil {
				log.Printf("Dropped error: %v\n", err)
			}
		}
		//send pkgen
		go func() {
			defer droperr()
			err := w.Send(PkgenMessage{})
			if err != nil {
				errch <- err
			}
		}()
		//setup reading
		rch := make(chan Message)
		defer close(rch)
		go func() {
			defer droperr()
			for {
				m, err := r.NextMessage()
				if err != nil {
					errch <- err
					return
				}
				rch <- m
			}
		}()
		//tend to channels
		for {
			select {
			case err = <-errch:
				return err
			case m := <-rch:
				switch mv := m.(type) {
				case FileRequestMessage:
					go func() {
						defer droperr()
						err := flh.Handle(mv)
						if err != nil {
							errch <- err
						}
					}()
				case PackageRequestMessage:
					go func() {
						defer droperr()
						pws := w.Stream(mv.Stream)
						defer pws.Close()
						err := gpkg(mv.Name, pws)
						if err != nil {
							errch <- err
						}
					}()
				case DatMessage, StreamDoneMessage:
					err := r.HandleDat(mv)
					if err != nil {
						return err
					}
				case LogMessage:
					logch <- mv
				case PackagesReadyMessage:
					go func() {
						defer droperr()
						for _, p := range mv {
							//allocate stream
							strn, strr := r.Stream(1)
							//send request for package
							err := w.Send(PackageRequestMessage{
								Name:   p,
								Stream: strn,
							})
							if err != nil {
								strr.Close()
								errch <- err
								return
							}
							//download
							err = func() (err error) {
								defer func() {
									e := recover()
									if e != nil {
										err = e.(error)
									}
								}()
								return wpkg(p, strr)
							}()
							strr.Close() //force close stream
							if err != nil {
								errch <- err
								return
							}
						}
						//tell server that we are done transferring
						err := w.Send(DoneMessage{})
						if err != nil {
							errch <- err
							return
						}
					}()
				case ErrorMessage:
					return mv
				case DoneMessage:
					return nil
				}
			}
		}
	})
}
