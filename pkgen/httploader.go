package pkgen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

type httpLoader struct {
	cli    *http.Client
	maxbuf uint
}

var hprotos = []string{"http", "https"}

func (hl *httpLoader) SupportedProtocols() ([]string, error) {
	return hprotos, nil
}
func (hl *httpLoader) Get(u *url.URL) (io.ReadCloser, error) {
	shasum := u.Query().Get("sha256sum")
	switch u.Scheme { //check that the scheme is supported
	case "http":
		if shasum == "" { //insecure resource, needs hash
			return nil, ErrMissingHash
		}
	case "https":
	default:
		return nil, ErrUnsupportedProtocol
	}
	var shs []byte
	if shasum != "" {
		sum, err := hex.DecodeString(shasum) //decode sha256sum from hex
		if err != nil {
			return nil, err
		}
		if len(sum) != sha256.Size { //check that it is the right length
			return nil, errors.New("invalid hash: wrong length")
		}
		shs = sum
	}
	resp, err := hl.cli.Get(u.String())
	if err != nil {
		return nil, err
	}
	if shs != nil { //it is hashed, download to memory and verify
		if resp.ContentLength > int64(hl.maxbuf) {
			return nil, ErrExceedsMaxBuffer
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
			return nil, err
		}
		if !bytes.Equal(h.Sum(nil), shs) {
			return nil, errors.New("hash mismatch")
		}
		return ioutil.NopCloser(buf), nil
	}
	return resp.Body, nil
}

//maxReader is a reader that returns ErrExceedsMaxBuffer if too much is read
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

//NewHTTPLoader returns a new Loader which loads content over HTTP
//client is the HTTP client to use to make the requests
//if client is nil, it will use http.DefaultClient
//maxbuf is the maximum number of bytes to buffer in memory when necessary
//data will only be buffered in memory when there is an attached hash
func NewHTTPLoader(client *http.Client, maxbuf uint) Loader {
	if client == nil {
		client = http.DefaultClient
	}
	return &httpLoader{
		cli:    client,
		maxbuf: maxbuf,
	}
}
