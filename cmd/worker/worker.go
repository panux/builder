package main

import (
	"bufio"
	"bytes"
	"context"
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
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/panux/builder/internal"
	"github.com/panux/builder/pkgen/buildlog"
)

// authk is the authentication public key.
var authk []byte

// ctx is the server-wide context with cancellation.
var ctx context.Context

func main() {
	defer log.Println("Shutdown complete.")

	//waitgroup for background goroutines
	var wg sync.WaitGroup
	defer wg.Wait()

	//set up server-wide context
	var srvcancel context.CancelFunc
	ctx, srvcancel = context.WithCancel(context.Background())
	sigch := make(chan os.Signal, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-sigch
		log.Println("Initiating shutdown")
		srvcancel()
	}()
	signal.Notify(sigch, syscall.SIGTERM)

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

	//prepare auth
	var err error
	authk, err = loadAuthKey(authkey)
	if err != nil {
		log.Fatalf("failed to load auth key: %q\n", err.Error())
	}

	//http setup
	http.HandleFunc("/mkdir", handleMkdir)
	http.HandleFunc("/write", handleWriteFile)
	http.HandleFunc("/read", handleReadFile)
	http.HandleFunc("/run", handleRunCmd)
	http.HandleFunc("/status", handleStatus)

	//run http servers
	srv := &http.Server{
		Addr: addr,
	}
	srv2 := &http.Server{
		Addr: ":80",
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := srv.ListenAndServeTLS(tlscertpath, tlskeypath)
		if err != nil {
			log.Printf("HTTPS server crashed: %q\n", err.Error())
			srvcancel() //shutdown
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := srv2.ListenAndServe()
		if err != nil {
			log.Printf("HTTP server crashed: %q\n", err.Error())
			srvcancel() //shutdown
		}
	}()

	//do http server shutdowns
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		sctx, _ := context.WithTimeout(context.Background(), time.Second*15)
		srv.Shutdown(sctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		sctx, _ := context.WithTimeout(context.Background(), time.Second*15)
		srv2.Shutdown(sctx)
	}()

	//wait for server to be shut down
	<-ctx.Done()
}

// loadAuthKey reads an authentication key and decodes it with PEM.
func loadAuthKey(path string) ([]byte, error) {
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(dat)
	return blk.Bytes, nil
}

// errAccessDenied is an error indicating that the request did not have proper authentication.
var errAccessDenied = errors.New("access denied")

// authReq decodes a request and checks the authentication validity.
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

// handleStatus handles Kubernetes status requests.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("online"))
}

// handleMkdir handles mkdir requests.
func handleMkdir(w http.ResponseWriter, r *http.Request) {
	//check HTTP method
	if r.Method != http.MethodPost {
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		log.Println("mkdir: unsupported method")
		return
	}

	//parse request
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("form parse error: %q", err.Error()), http.StatusBadRequest)
		log.Printf("mkdir form parse error: %q\n", err.Error())
		return
	}
	reqs := r.FormValue("request")
	if reqs == "" {
		http.Error(w, "mkdir: missing request", http.StatusBadRequest)
		return
	}
	req, err := authReq(reqs, &internal.MkdirRequest{})
	if err != nil {
		if err == errAccessDenied {
			http.Error(w, "access denied", http.StatusUnauthorized)
		} else {
			http.Error(w, fmt.Sprintf("failed to decode request: %q", err.Error()), http.StatusBadRequest)
		}
		log.Printf("mkdir: request error: %q\n", err.Error())
		return
	}

	//execute request
	mkreq := req.Request.(*internal.MkdirRequest)
	if mkreq.Parent {
		err = os.MkdirAll(mkreq.Dir, 0644)
	} else {
		err = os.Mkdir(mkreq.Dir, 0644)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to mkdir: %q", err.Error()), http.StatusInternalServerError)
		return
	}

	//write an OK
	w.WriteHeader(http.StatusOK)
}

// handleWriteFile handles a file write request.
func handleWriteFile(w http.ResponseWriter, r *http.Request) {
	//WaitGroup for cleanup
	var wg sync.WaitGroup
	defer wg.Wait()

	//check request method
	if r.Method != http.MethodPost {
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		log.Println("writeFile: unsupported method")
		return
	}

	//read request
	br := bufio.NewReader(r.Body)
	reqdat, err := br.ReadBytes(0)
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		log.Printf("writeFile failed to read request: %q\n", err.Error())
		return
	}
	req, err := authReq(string(reqdat), &internal.FileWriteRequest{})
	if err != nil {
		if err == errAccessDenied {
			http.Error(w, "access denied", http.StatusUnauthorized)
		} else {
			http.Error(w, fmt.Sprintf("failed to decode request: %q", err.Error()), http.StatusBadRequest)
		}
		log.Printf("writeFile failed to decode request: %q\n", err.Error())
		return
	}
	fwreq := req.Request.(*internal.FileWriteRequest)

	//open file
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

	//handle cancellation
	fin := make(chan struct{})
	defer close(fin)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			f.Close()
		case <-fin:
		}
	}()

	//store file
	_, err = io.Copy(f, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("write error: %q", err.Error()), http.StatusInternalServerError)
		return
	}
}

// handleReadFile handles file read requests.
func handleReadFile(w http.ResponseWriter, r *http.Request) {
	//WaitGroup for cleanup
	var wg sync.WaitGroup
	defer wg.Wait()

	//check request method
	if r.Method != http.MethodPost {
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		log.Println("readFile unsupported method")
		return
	}

	//parse request
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("form parse error: %q", err.Error()), http.StatusBadRequest)
		log.Printf("readFile form parse error: %q\n", err.Error())
		return
	}
	reqs := r.FormValue("request")
	if reqs == "" {
		http.Error(w, "missing request", http.StatusBadRequest)
		log.Println("missing request")
		return
	}
	req, err := authReq(reqs, &internal.FileReadRequest{})
	if err != nil {
		if err == errAccessDenied {
			http.Error(w, "access denied", http.StatusUnauthorized)
		} else {
			http.Error(w, fmt.Sprintf("failed to decode request: %q", err.Error()), http.StatusBadRequest)
		}
		log.Printf("readFile request error: %q\n", err.Error())
		return
	}
	frreq := req.Request.(*internal.FileReadRequest)

	//open file
	f, err := os.Open(frreq.Path)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("failed to open file: %q", err.Error()), http.StatusInternalServerError)
		}
		log.Printf("readFile file open error: %q\n", err.Error())
		return
	}
	defer f.Close()

	//handle cancellation
	fin := make(chan struct{})
	defer close(fin)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			f.Close()
		case <-fin:
		}
	}()

	//send file
	io.Copy(w, f)
}

// wsup is a websocket upgrader used by handleRunCmd.
var wsup = &websocket.Upgrader{
	HandshakeTimeout: time.Second * 30,
}

// handleRunCmd handles command run requests.
func handleRunCmd(w http.ResponseWriter, r *http.Request) {
	//upgrade request to websocket
	c, err := wsup.Upgrade(w, r, nil)
	if err != nil {
		return //error sent by Upgrade
	}
	defer func() {
		cerr := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if cerr != nil {
			c.Close()
			if err == nil {
				err = cerr
			}
			return
		}
		time.Sleep(time.Second)
		cerr = c.Close()
		if err == nil {
			err = cerr
		}
	}()

	//decode request
	req, err := readWSReq(c, new(internal.CommandRequest))
	if err != nil {
		log.Printf("bad cmd request: %q\n", err.Error())
		return
	}
	cmdr := req.Request.(*internal.CommandRequest)
	log.Printf("accepted new command request: %v\n", cmdr)

	//prepare command
	cmd := exec.CommandContext(ctx, cmdr.Argv[0], cmdr.Argv[1:]...)
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

	//prepare logging
	lh := buildlog.NewMutexedLogHandler(&wsLogHandler{c: c})
	lh = buildlog.NewMultiLogHandler(lh, buildlog.DefaultHandler)
	defer lh.Close()
	if cmdr.EnableStdin {
		cmd.Stdin = internal.NewWebsocketReader(c)
	}
	if !cmdr.DisableStdout {
		w := buildlog.NewLogWriter(lh, buildlog.StreamStdout)
		defer w.Close()
		cmd.Stdout = w
	}
	if !cmdr.DisableStderr {
		w := buildlog.NewLogWriter(lh, buildlog.StreamStderr)
		defer w.Close()
		cmd.Stderr = w
	}

	//execute command
	err = cmd.Run()

	//send log termination message
	if err != nil {
		lh.Log(buildlog.Line{
			Text:   fmt.Sprintf("error: %q", err.Error()),
			Stream: buildlog.StreamMeta,
		})
	} else {
		lh.Log(buildlog.Line{
			Text:   "success",
			Stream: buildlog.StreamMeta,
		})
	}
}

// wsLogHandler is a buildlog.Handler used to send logs over a websocket.
type wsLogHandler struct {
	c *websocket.Conn
}

func (wsl *wsLogHandler) Log(ll buildlog.Line) error {
	return wsl.c.WriteJSON(ll)
}

func (wsl *wsLogHandler) Close() error {
	return nil
}

// readWSReq reads, decodes, and authentiates a request from a websocket.
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
