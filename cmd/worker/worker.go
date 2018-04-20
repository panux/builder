package main

import (
	"bufio"
	"bytes"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/gorilla/websocket"
	"github.com/panux/builder/internal"
	"github.com/panux/builder/pkgen/worker"
)

var authk []byte //authentication public key

func main() {
	//parse flags
	var tlskeypath string
	var tlscertpath string
	var authkey string
	var addr string
	flag.StringVar(&tlskeypath, "key", "/srv/secret/srvkey.pem", "server TLS private key")
	flag.StringVar(&tlscertpath, "cert", "/srv/secret/cert.pem", "server TLS certificate")
	flag.StringVar(&authkey, "auth", "/srv/secret/auth.pem", "public key to verify requests")
	flag.StringVar(&addr, "https", ":443", "https server port")
	flag.Parse()

	//run http server
	log.Fatalf("Failed to listen: %q\n", http.ListenAndServeTLS(addr, tlskeypath, tlscertpath, nil))
}

func loadAuthKey(path string) ([]byte, error) {
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(dat)
	return blk.Bytes, nil
}

var errAccessDenied = errors.New("access denied")

func authReq(raw string, reqsub interface{}) (*internal.Request, error) {
	req, err := internal.DecodeRequest(raw, reqsub)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(req.PublicKey, authk) {
		return nil, errAccessDenied
	}
	return req, nil
}

func handleMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "unsupported method", http.StatusNotImplemented)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("form parse error: %q", err.Error()), http.StatusBadRequest)
		return
	}
	reqs := r.FormValue("request")
	if reqs == "" {
		http.Error(w, "missing request", http.StatusBadRequest)
		return
	}
	req, err := authReq(reqs, internal.MkdirRequest{})
	if err != nil {
		if err == errAccessDenied {
			http.Error(w, "access denied", http.StatusUnauthorized)
		} else {
			http.Error(w, fmt.Sprintf("failed to decode request: %q", err.Error()), http.StatusBadRequest)
		}
		return
	}

	mkreq := req.Request.(internal.MkdirRequest)
	if mkreq.Parent {
		err = os.MkdirAll(mkreq.Dir, 0644)
	} else {
		err = os.Mkdir(mkreq.Dir, 0644)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to mkdir: %q", err.Error()), http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
}
func handleWriteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "unsupported method", http.StatusNotImplemented)
		return
	}

	br := bufio.NewReader(r.Body)
	reqdat, err := br.ReadBytes(0)
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}
	req, err := authReq(string(reqdat), internal.FileWriteRequest{})
	if err != nil {
		if err == errAccessDenied {
			http.Error(w, "access denied", http.StatusUnauthorized)
		} else {
			http.Error(w, fmt.Sprintf("failed to decode request: %q", err.Error()), http.StatusBadRequest)
		}
		return
	}
	fwreq := req.Request.(internal.FileWriteRequest)

	f, err := os.OpenFile(fwreq.Path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open file: %q", err.Error()), http.StatusInternalServerError)
		return
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil && err == nil {
			http.Error(w, fmt.Sprintf("failed to close: %q", cerr.Error()), http.StatusInternalServerError)
		}
	}()

	_, err = io.Copy(f, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("write error: %q", err.Error()), http.StatusInternalServerError)
		return
	}
}
func handleReadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "unsupported method", http.StatusNotImplemented)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("form parse error: %q", err.Error()), http.StatusBadRequest)
		return
	}
	reqs := r.FormValue("request")
	if reqs == "" {
		http.Error(w, "missing request", http.StatusBadRequest)
		return
	}
	req, err := authReq(reqs, internal.FileReadRequest{})
	if err != nil {
		if err == errAccessDenied {
			http.Error(w, "access denied", http.StatusUnauthorized)
		} else {
			http.Error(w, fmt.Sprintf("failed to decode request: %q", err.Error()), http.StatusBadRequest)
		}
		return
	}
	frreq := req.Request.(internal.FileReadRequest)

	r.URL.Path = "" //tell ServeFile to ignore path in request
	http.ServeFile(w, r, frreq.Path)
}

var wsup = &websocket.Upgrader{ //websocket upgrader for handleRunCmd
	HandshakeTimeout: time.Second * 30,
}

func handleRunCmd(w http.ResponseWriter, r *http.Request) {
	c, err := wsup.Upgrade(w, r, nil)
	if err != nil {
		return //error sent by Upgrade
	}
	defer c.Close()

	req, err := readWSReq(c, internal.CommandRequest{})
	if err != nil {
		return
	}
	cmdr := req.Request.(internal.CommandRequest)

	cmd := exec.Command(cmdr.Argv[0], cmdr.Argv[1:]...)
	cmd.Dir = "/"
	if cmdr.Env != nil {
		env := make([]string, len(cmdr.Env))
		i := 0
		for k, v := range cmdr.Env {
			env[i] = fmt.Sprintf("%s=%s", k, v)
			i++
		}
		sort.Strings(env)
		cmd.Env = env
	}

	lh := worker.NewMutexedLogHandler(&wsLogHandler{c: c})
	if cmdr.EnableStdin {
		cmd.Stdin = internal.NewWebsocketReader(c)
	}
	if !cmdr.DisableStdout {
		w := worker.NewLogWriter(lh, worker.StreamStdout)
		defer w.Close()
		cmd.Stdout = w
	}
	if !cmdr.DisableStderr {
		w := worker.NewLogWriter(lh, worker.StreamStderr)
		defer w.Close()
		cmd.Stderr = w
	}

	err = cmd.Run()

	if err != nil {
		lh.Log(worker.LogLine{
			Text:   fmt.Sprintf("error: %q", err.Error()),
			Stream: worker.StreamMeta,
		})
	} else {
		lh.Log(worker.LogLine{
			Text:   "success",
			Stream: worker.StreamMeta,
		})
	}
}

type wsLogHandler struct {
	c *websocket.Conn
}

func (wsl *wsLogHandler) Log(ll worker.LogLine) error {
	return wsl.c.WriteJSON(ll)
}

func (wsl *wsLogHandler) Close() error {
	return nil
}

func readWSReq(c *websocket.Conn, reqsub interface{}) (*internal.Request, error) {
	mt, r, err := c.NextReader()
	if err != nil {
		return nil, err
	}
	switch mt {
	case websocket.TextMessage:
	case websocket.BinaryMessage:
		return nil, errors.New("bad message type")
	}
	dat, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return authReq(string(dat), reqsub)
}
