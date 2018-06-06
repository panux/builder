package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/panux/builder/internal"
	"github.com/panux/builder/pkgen/buildlog"
)

// Status sends a status request to the worker.
func (w *Worker) Status(ctx context.Context) (str string, err error) {
	//calculate get URL
	u, err := w.u.Parse("/status")
	if err != nil {
		return
	}

	//prepare request
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req = req.WithContext(ctx)

	//send request
	resp, err := w.hcl.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New(resp.Status)
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil && err == nil {
			err = cerr
			str = ""
		}
	}()

	//read response
	dat, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(dat), nil
}

// Mkdir makes a directory on the worker.
// If mkparent is true, it will create parent directories.
func (w *Worker) Mkdir(ctx context.Context, path string, mkparent bool) (err error) {
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
	req, err := http.NewRequest(http.MethodPost, u.String(), nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Form = url.Values{
		"request": []string{string(rdat)},
	}
	resp, err := w.hcl.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	if resp.StatusCode != http.StatusOK {
		dat, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.New(string(dat))
	}

	//discard response
	_, err = io.Copy(ioutil.Discard, resp.Body)
	return
}

// fileWReader is the reader used in WriteFile.
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

// WriteFile writes a file on the worker.
// Data is copied from the io.Reader src.
func (w *Worker) WriteFile(ctx context.Context, path string, src io.Reader) (err error) {
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
	req, err := http.NewRequest(http.MethodPost, u.String(), fwr)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := w.hcl.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}

	//discard response
	_, err = io.Copy(ioutil.Discard, resp.Body)
	return
}

// ReadFile reads a file from the worker.
// The file contents are copied into dst.
func (w *Worker) ReadFile(ctx context.Context, path string, dst io.Writer) (err error) {
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
	req, err := http.NewRequest(http.MethodPost, u.String(), nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Form = url.Values{
		"request": []string{string(rdat)},
	}
	resp, err := w.hcl.Do(req)
	if err != nil {
		return
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}

	//copy file data
	_, err = io.Copy(dst, resp.Body)
	return
}

// CmdOptions is a set of options for running a command.
// All fields are optional.
type CmdOptions struct {
	// Env is a set of environment variables to use when running the command.
	Env map[string]string

	// If DisableStdout is true then stdout will not be logged.
	DisableStdout bool

	// If DisableStderr is true then stdout will not be logged.
	DisableStderr bool

	// LogOut is the LogHandler used for output.
	// Defaults to DefaultLogHandler.
	LogOut buildlog.Handler
}

// defaults sets defaults where missing.
func (c CmdOptions) defaults() CmdOptions {
	if c.LogOut == nil {
		c.LogOut = buildlog.DefaultHandler
	}
	return c
}

// ErrCmdFail is an error indicating that a command failed.
var ErrCmdFail = errors.New("command did not report success")

// RunCmd runs a command on the worker.
// If stdin is not set then stdin will not be connected.
func (w *Worker) RunCmd(ctx context.Context, argv []string, stdin io.Reader, opts CmdOptions) (err error) {
	//waitgroup
	var wg sync.WaitGroup
	defer wg.Wait()

	//fill in blanks with defaults
	opts = opts.defaults()

	//enforce thread safety
	opts.LogOut = buildlog.NewMutexedLogHandler(opts.LogOut)

	//check for success message
	var success bool
	opts.LogOut = buildlog.InterceptMeta(opts.LogOut, func(s string) {
		if s == "success" {
			success = true
		} else { //forward error to stderr
			opts.LogOut.Log(buildlog.Line{
				Text:   s,
				Stream: buildlog.StreamStderr,
			})
		}
	})
	defer func() {
		if success {
			err = nil
		}
		if err == nil && !success {
			err = ErrCmdFail
		}
	}()

	//calculate target URL
	u, err := w.u.Parse("/run")
	if err != nil {
		return
	}
	u.Scheme = "wss"

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

	//start cancellation w/ close
	fin := make(chan struct{})
	defer close(fin)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			c.Close()
		case <-fin:
		}
	}()

	//write request to websocket
	err = c.WriteMessage(websocket.TextMessage, rdat)
	if err != nil {
		return
	}

	//do background stdin copy
	if stdin != nil {
		swriter, err := c.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
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
		case websocket.TextMessage:
			var ll buildlog.Line
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
