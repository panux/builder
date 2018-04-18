//Package worker is a client package for the build worker
package worker

import (
	"crypto/rsa"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/panux/builder/pkgen/worker/request"
)

//Worker is a worker API client
//It is not concurrency safe
type Worker struct {
	u       *url.URL        //URL of worker
	hcl     *http.Client    //http client to use when making HTTP requests
	authkey *rsa.PrivateKey //key to sign request with
	pod     *workerPod      //worker pod kubernetes data
}

//Close closes a worker (killing the pod and deleting the SSL secret)
func (w *Worker) Close() error {
	if w.pod == nil {
		return io.ErrClosedPipe
	}
	err := w.pod.Close()
	if err != nil {
		return err
	}
	w.pod = nil
	return nil
}

//Mkdir makes a directory on the worker
func (w *Worker) Mkdir(path string) (err error) {
	//calculate post URL
	u, err := w.u.Parse("/mkdir")
	if err != nil {
		return
	}

	//prepare request
	rdat, err := (&request.Request{}).Sign(w.authkey)
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
