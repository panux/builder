package worker

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/panux/builder/pkgen/worker/internal"
)

//Mkdir makes a directory on the worker.
//If mkparent is true, it will create parent directories.
func (w *Worker) Mkdir(path string, mkparent bool) (err error) {
	//calculate post URL
	u, err := w.u.Parse("/mkdir")
	if err != nil {
		return
	}

	//prepare request
	rdat, err := (&internal.Request{
		APIVersion: internal.APIVersion,
		Request: internal.MkdirRequest{
			Dir:    path,
			Parent: mkparent,
		},
	}).Sign(w.authkey)
	if err != nil {
		return
	}

	//send post request
	resp, err := w.hcl.PostForm(u.String(), url.Values{
		"request": []string{string(rdat)},
	})
	if err != nil {
		return
	}

	//discard response
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(ioutil.Discard, resp.Body)
	return
}

//reader used in WriteFile
type fileWReader struct {
	header []byte
	r      io.Reader
}

func (fwr *fileWReader) Read(dat []byte) (int, error) {
	if len(fwr.header) == 0 {
		return fwr.r.Read(dat)
	}
	n := copy(dat, fwr.header)
	fwr.header = fwr.header[n:]
	return n, nil
}

//WriteFile writes a file on the worker.
//Data is copied from the io.Reader src.
func (w *Worker) WriteFile(path string, src io.Reader) (err error) {
	//calculate post URL
	u, err := w.u.Parse("/write")
	if err != nil {
		return
	}

	//prepare request
	rdat, err := (&internal.Request{
		APIVersion: internal.APIVersion,
		Request: internal.FileWriteRequest{
			Path: path,
		},
	}).Sign(w.authkey)
	if err != nil {
		return
	}
	rdat = append(rdat, 0) //add null terminator

	//send post request
	fwr := &fileWReader{
		header: rdat,
		r:      src,
	}
	resp, err := w.hcl.Post(u.String(), "application/octet-stream", fwr)
	if err != nil {
		return
	}

	//discard response
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(ioutil.Discard, resp.Body)
	return
}

//ReadFile reads a file from the worker.
//The file contents are copied into dst.
func (w *Worker) ReadFile(path string, dst io.Writer) (err error) {
	//calculate post URL
	u, err := w.u.Parse("/read")
	if err != nil {
		return
	}

	//prepare request
	rdat, err := (&internal.Request{
		APIVersion: internal.APIVersion,
		Request: internal.FileReadRequest{
			Path: path,
		},
	}).Sign(w.authkey)
	if err != nil {
		return
	}

	//send post request
	resp, err := w.hcl.PostForm(u.String(), url.Values{
		"request": []string{string(rdat)},
	})
	if err != nil {
		return
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()

	//copy file data
	_, err = io.Copy(dst, resp.Body)
	return
}

//CmdOptions is a set of options for running a command.
//All fields are optional.
type CmdOptions struct {
	//Env is a set of environment variables to use when running the command
	Env map[string]string

	//If DisableStdout is true then stdout will not be logged
	DisableStdout bool

	//If DisableStderr is true then stdout will not be logged
	DisableStderr bool

	//LogOut is the LogHandler used for output.
	//Defaults to DefaultLogHandler.
	LogOut LogHandler
}

//set defaults where missing
func (c CmdOptions) defaults() CmdOptions {
	if c.LogOut == nil {
		c.LogOut = DefaultLogHandler
	}
	return c
}

//RunCmd runs a command on the worker.
//If stdin is not set then stdin will not be connected.
func (w *Worker) RunCmd(argv []string, stdin io.Reader, opts CmdOptions) (err error) {
	//fill in blanks with defaults
	opts = opts.defaults()

	//calculate target URL
	u, err := w.u.Parse("/run")
	if err != nil {
		return
	}

	//prepare request
	rdat, err := (&internal.Request{
		APIVersion: internal.APIVersion,
		Request: internal.CommandRequest{
			Argv:          argv,
			Env:           opts.Env,
			EnableStdin:   stdin != nil,
			DisableStdout: opts.DisableStdout,
			DisableStderr: opts.DisableStderr,
		},
	}).Sign(w.authkey)
	if err != nil {
		return
	}

	//dial websocket
	c, _, err := w.wscl.Dial(u.String(), nil)
	if err != nil {
		return
	}
	defer func() {
		cerr := c.Close()
		if cerr != nil && err != nil {
			err = cerr
		}
	}()

	//write request to websocket
	err = c.WriteMessage(websocket.BinaryMessage, rdat)
	if err != nil {
		return
	}

	//do background stdin copy
	if stdin != nil {
		swriter, err := c.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return err
		}
		go func() {
			io.Copy(swriter, stdin)
			defer swriter.Close()
		}()
	}

	//loop over incoming messages
	for {
		mt, dat, err := c.ReadMessage()
		if err != nil {
			return err
		}
		switch mt {
		case websocket.CloseMessage:
			return nil
		case websocket.BinaryMessage:
			var ll LogLine
			err = json.Unmarshal(dat, &ll)
			if err != nil {
				return err
			}
			err = opts.LogOut.Log(ll)
			if err != nil {
				return err
			}
		}
	}
}
