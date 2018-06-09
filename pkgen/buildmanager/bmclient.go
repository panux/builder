// Package buildmanager is a client for the buildmanager server.
// It can be used to build packages in the cluster.
package buildmanager

import (
	"archive/tar"
	"context"
	"crypto/rsa"
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

// Client is a client to the buildmanager server.
type Client struct {
	// u is the base URL to send requests to.
	u *url.URL

	// authk is the private key to use to sign requests.
	authk *rsa.PrivateKey

	// wscl is the websocket dialer to use to perform websocket requests.
	wscl *websocket.Dialer
}

// NewClient creates a new Client for the server at the provided URL.
// It uses the provided dialer, or defaults to websocket.DefaultDialer if nil.
func NewClient(u *url.URL, auth *rsa.PrivateKey, dial *websocket.Dialer) *Client {
	if dial == nil {
		dial = websocket.DefaultDialer
	}
	return &Client{
		u:     u,
		authk: auth,
		wscl:  dial,
	}
}

// BuildOptions is a set of options for Build.
type BuildOptions struct {
	// Out is a function which is called to write the output packages.
	// Required.
	Out func(name string, r io.Reader) error

	// LogOut is the buildlog.Handler used for log output.
	// Not closed on completion.
	LogOut buildlog.Handler

	// Context is a contest for cancellation.
	// Optional. Defaults to context.Background.
	Context context.Context
}

// Status runs a status probe on the server.
func (cli *Client) Status() error {
	//determine request URL
	u, err := cli.u.Parse("/status")
	if err != nil {
		return err
	}
	u = cli.u.ResolveReference(u)

	//send request
	resp, err := http.Get(u.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

// Build builds a package.
func (cli *Client) Build(bjr *BuildJobRequest, opts BuildOptions) (err error) {
	//determine request URL
	u, err := cli.u.Parse("/build")
	if err != nil {
		return
	}
	u.Scheme = "ws"

	//prepare request
	rdat, err := (&internal.Request{
		APIVersion: internal.APIVersion,
		Request: internal.BuildRequest{
			Pkgen: bjr.pk,
		},
	}).Sign(cli.authk)
	if err != nil {
		return
	}

	//connect to server
	c, _, err := cli.wscl.Dial(u.String(), nil)
	if err != nil {
		return
	}
	defer func() {
		cerr := c.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()

	//handle errors from goroutines
	var cancelled bool
	var procreaderr error
	defer func() {
		if cancelled {
			err = context.Canceled
		}
		if procreaderr != nil && err != nil {
			err = procreaderr
		}
	}()
	//do WaitGroup
	var wg sync.WaitGroup
	defer wg.Wait()

	//setup context
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	//handle cancellation
	stopch := make(chan struct{})
	defer close(stopch)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			c.Close() //close to cancel I/O
			cancelled = true
		case <-stopch:
		}
	}()

	//start processing output in background
	wg.Add(1)
	go procWsRead(c, opts, &wg, &procreaderr)

	//send request
	err = wsSendRequest(c, rdat)
	if err != nil {
		return
	}

	//send packages
	if !bjr.pk.Builder.IsBootstrap() {
		err = wsSendPackages(c, bjr)
		if err != nil {
			return
		}
	}

	//send souce tar
	err = wsSendSources(ctx, c, bjr)
	if err != nil {
		return
	}

	return
}

// wsSendRequest sends a request over a websocket.
func wsSendRequest(c *websocket.Conn, r []byte) (err error) {
	w, err := c.NextWriter(websocket.TextMessage)
	if err != nil {
		return
	}
	defer func() {
		cerr := w.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = w.Write(r)
	return
}

// procWsRead processes a stream of websocket reads.
func procWsRead(c *websocket.Conn, opts BuildOptions, wg *sync.WaitGroup, e *error) {
	defer wg.Done()
	var err error
	defer func() {
		*e = err
	}()
	for {
		err = wsDoRead(c, opts)
		if err == io.EOF {
			err = nil
			return
		}
		if err != nil {
			return
		}
	}
}

// wsDoRead handles a websocket read.
func wsDoRead(c *websocket.Conn, opts BuildOptions) error {
	mt, r, err := c.NextReader()
	if err != nil {
		return err
	}
	switch mt {
	case websocket.BinaryMessage:
		tr := tar.NewReader(r)
		for {
			h, err := tr.Next()
			if err != nil {
				return err
			}
			fi := h.FileInfo()
			opts.Out(fi.Name(), io.LimitReader(tr, fi.Size()))
		}
	case websocket.TextMessage:
		var line buildlog.Line
		err = json.NewDecoder(r).Decode(&line)
		if err != nil {
			return err
		}
		if line.Stream == buildlog.StreamMeta && line.Text == "success" {
			return io.EOF
		}
		err = opts.LogOut.Log(line)
		if err != nil {
			return err
		}
	}
	return nil
}

// wsSendPackages sends the packages required for the build.
func wsSendPackages(c *websocket.Conn, bjr *BuildJobRequest) (err error) {
	w, err := c.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	defer func() {
		cerr := w.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	err = bjr.tar(w)
	return
}

// wsSendSources sends a tar of the sources.
func wsSendSources(ctx context.Context, c *websocket.Conn, bjr *BuildJobRequest) (err error) {
	w, err := c.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	defer func() {
		cerr := w.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	err = bjr.writeSourceTar(ctx, w)
	return
}
