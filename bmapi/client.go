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

//Build runs a build using the BuildManager
//Log output will be sent to logch (which is closed afterward)
//A slow reader on logch may slow the build process
//wpkg is a function used to store the output packages
//wpkg takes 2 arguments: the name of the package, then an io.ReadCloser of the package (in .tar.gz format)
//if wpkg returns an error, it will be propogated to the error of the Build function
//gpkg is a function called to load a dependent package
//arguments for gpkg are like wpkg but with a writer, and error handling is the same
func (cli *Client) Build(pk *pkgen.PackageGenerator, logch chan<- LogMessage, fs vfs.FileSystem, wpkg func(string, io.ReadCloser) error, gpkg func(string, io.WriteCloser) error) error {
	//genetate request URL
	ru, err := url.Parse("/status")
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
		_ = flh
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
