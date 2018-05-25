package pkgen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

// httpLoader is a Loader which loads data over HTTP
type httpLoader struct {
	cli    *http.Client
	maxbuf uint
}

// hprotos is the set of protocols supported by httpLoader
var hprotos = []string{"http", "https"}

func (hl *httpLoader) SupportedProtocols() ([]string, error) {
	return hprotos, nil
}

func (hl *httpLoader) Get(ctx context.Context, u *url.URL) (int64, io.ReadCloser, error) {
	shasum := u.Query().Get("sha256sum")

	//check that the scheme is supported
	switch u.Scheme {
	case "http":
		if shasum == "" { //insecure resource, needs hash
			return -1, nil, ErrMissingHash
		}
	case "https":
	default:
		return -1, nil, ErrUnsupportedProtocol
	}

	//decode hash if present
	var shs []byte
	if shasum != "" {
		sum, err := hex.DecodeString(shasum) //decode sha256sum from hex
		if err != nil {
			return -1, nil, err
		}
		if len(sum) != sha256.Size { //check that it is the right length
			return -1, nil, errors.New("invalid hash: wrong length")
		}
		shs = sum
	}

	//prepare get request
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return -1, nil, err
	}
	req = req.WithContext(ctx)
	resp, err := hl.cli.Do(req)
	if err != nil {
		return -1, nil, err
	}

	//if it is hashed, download to memory and verify
	if shs != nil {
		if resp.ContentLength > int64(hl.maxbuf) {
			return -1, nil, ErrExceedsMaxBuffer
		}
		defer resp.Body.Close()
		buf := bytes.NewBuffer(nil)
		h := sha256.New()
		t := io.TeeReader(resp.Body, h)
		mrt := new(maxReader)
		mrt.n = hl.maxbuf
		mrt.r = t
		_, err = io.Copy(buf, mrt)
		if err != nil {
			return -1, nil, err
		}
		if !bytes.Equal(h.Sum(nil), shs) {
			return -1, nil, errors.New("hash mismatch")
		}
		return resp.ContentLength, ioutil.NopCloser(buf), nil
	}

	//return regular response
	return resp.ContentLength, resp.Body, nil
}

// maxReader is a reader that returns ErrExceedsMaxBuffer if too much is read
type maxReader struct {
	r io.Reader
	n uint
}

func (mr *maxReader) Read(dat []byte) (int, error) {
	n, err := mr.Read(dat)
	if uint(n) > mr.n {
		return n, ErrExceedsMaxBuffer
	}
	mr.n -= uint(n)
	return n, err
}

// NewHTTPLoader returns a new Loader which loads content over HTTP
// client is the HTTP client to use to make the requests
// if client is nil, it will use http.DefaultClient
// maxbuf is the maximum number of bytes to buffer in memory when necessary
// data will only be buffered in memory when there is an attached hash
func NewHTTPLoader(client *http.Client, maxbuf uint) Loader {
	if client == nil {
		client = http.DefaultClient
	}
	return &httpLoader{
		cli:    client,
		maxbuf: maxbuf,
	}
}
