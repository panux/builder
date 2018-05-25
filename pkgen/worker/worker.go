//Package worker is a client package for the build worker
package worker

import (
	"crypto/rsa"
	"io"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

// Worker is a worker API client
// It is not concurrency safe
type Worker struct {
	u       *url.URL          //URL of worker
	hcl     *http.Client      //http client to use when making HTTP requests
	wscl    *websocket.Dialer //websocket client to use when making websocket requests
	authkey *rsa.PrivateKey   //key to sign request with
	pod     *workerPod        //worker pod kubernetes data
}

// Close closes a worker (killing the pod and deleting the SSL secret)
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
