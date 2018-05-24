//Package dlapi implements a client for github.com/panux/builder/cmd/dlserver
package dlapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/panux/builder/pkgen"
)

//DlClient is a client for a download server
type DlClient struct {
	cli  *http.Client
	base *url.URL
}

//NewDlClient returns a DlClient with the specified server URL and http client
//if client is null, it will use http.DefaultClient
func NewDlClient(u *url.URL, client *http.Client) *DlClient {
	if client == nil {
		client = http.DefaultClient
	}
	dlc := new(DlClient)
	dlc.base = u
	dlc.cli = client
	return dlc
}

//Status is the JSON sent by /status
type Status struct {
	Status  string `json:"status"` //should be "running"
	Version string `json:"version"`
}

//Status sends a request to /status
func (c *DlClient) Status() (*Status, error) {
	purl, err := url.Parse("/status")
	if err != nil {
		return nil, err
	}
	purl = c.base.ResolveReference(purl)
	resp, err := c.cli.Get(purl.String())
	if err != nil {
		return nil, err
	}
	var st Status
	err = json.NewDecoder(resp.Body).Decode(&st)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

//Get runs Loader.Get on the server (with cache)
func (c *DlClient) Get(ctx context.Context, u *url.URL) (int64, io.ReadCloser, error) {
	gurl, err := url.Parse("/get")
	if err != nil {
		return -1, nil, err
	}
	gurl.Query().Add("url", u.String())
	gurl = c.base.ResolveReference(gurl)
	req, err := http.NewRequest(http.MethodGet, gurl.String(), nil)
	if err != nil {
		return -1, nil, err
	}
	req = req.WithContext(ctx)
	resp, err := c.cli.Do(req)
	if err != nil {
		return -1, nil, err
	}
	return resp.ContentLength, resp.Body, nil
}

//SupportedProtocols implements Loader.SupportedProtocols
func (c *DlClient) SupportedProtocols() ([]string, error) {
	purl, err := url.Parse("/protos")
	if err != nil {
		panic(err)
	}
	purl = c.base.ResolveReference(purl)
	resp, err := c.cli.Get(purl.String())
	if err != nil {
		return nil, err
	}
	var protos []string
	err = json.NewDecoder(resp.Body).Decode(&protos)
	if err != nil {
		return nil, err
	}
	return protos, nil
}

var l pkgen.Loader = &DlClient{}
