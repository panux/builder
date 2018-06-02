package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/panux/builder/internal"
	"github.com/panux/builder/pkgen"
	"github.com/panux/builder/pkgen/buildlog"
	"github.com/panux/builder/pkgen/worker"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var starter *worker.Starter
var auth [][]byte
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
	var addr string
	var namespace string
	var authkeys string
	flag.StringVar(&addr, "http", ":80", "http listen address")
	flag.StringVar(&namespace, "namespace", "default", "Kubernetes namespace to run workers in")
	flag.StringVar(&authkeys, "auth", "/srv/authkeys.json", "JSON file containing authorized RSA auth keys")
	flag.Parse()

	//Prep Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Kubernetes in cluster config failed: %q\n", err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create ClientSet: %q\n", err.Error())
	}

	//Prep worker Starter
	starter = worker.NewStarter(clientset, namespace)

	//Load auth list
	loadAuthKeys(authkeys)

	//http
	http.HandleFunc("/build", handleBuild)
	http.HandleFunc("/status", handleStatus)
	srv := http.Server{
		Addr: addr,
	}
	wg.Add(1)
	go func() { //run http server in seperate goroutine
		defer wg.Done()
		err := srv.ListenAndServe()
		if err != nil {
			log.Printf("HTTP server crashed: %q\n", err.Error())
			srvcancel() //shutdown
		}
	}()

	<-ctx.Done() //wait for shutdown
}

// loadAuthKeys loads the authentication keys.
func loadAuthKeys(path string) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to open authkeys.json: %q\n", err.Error())
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&auth)
	if err != nil {
		log.Fatalf("Failed to unmarshal auth: %q\n", err.Error())
	}
}

// handleStatus handles status requests.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("online"))
}

// wsup is a websocket upgrader used for handleBuild.
var wsup = &websocket.Upgrader{
	HandshakeTimeout: time.Second * 30,
}

// handleBuild handles build requests (using websocket).
func handleBuild(w http.ResponseWriter, r *http.Request) {
	//upgrade to websocket
	c, err := wsup.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	//prep logger
	var l buildlog.Handler
	l = &wsLogHandler{c: c}

	//load request
	req, err := readWSReq(c, &internal.BuildRequest{})
	if err != nil {
		log.Printf("failed to read build request: %q\n", err.Error())
		l.Log(buildlog.Line{
			Text:   fmt.Sprintf("failed to read build request: %q\n", err.Error()),
			Stream: buildlog.StreamBuild,
		})
		l.Log(buildlog.Line{
			Text:   "failed",
			Stream: buildlog.StreamMeta,
		})
		return
	}
	br := req.Request.(*internal.BuildRequest)

	//start worker
	work, err := starter.Start(ctx, br.Pkgen)
	if err != nil {
		log.Printf("failed to start worker: %q\n", err.Error())
		return
	}
	defer func() {
		cerr := work.Close()
		if cerr != nil {
			log.Printf("failed to close worker: %q\n", err.Error())
			return
		}
	}()

	//install packages
	if br.Pkgen.Builder == "bootstrap" {
		if br.Pkgen.BuildDependencies != nil {
			err = work.RunCmd(
				ctx,
				append(
					[]string{"apk", "--no-cache", "add"},
					br.Pkgen.BuildDependencies...,
				),
				nil,
				worker.CmdOptions{LogOut: l},
			)
		}
	} else {
		err = doBootstrap(ctx, c, work, l)
	}
	if err != nil {
		l.Log(buildlog.Line{
			Text:   "package install failed",
			Stream: buildlog.StreamBuild,
		})
		l.Log(buildlog.Line{
			Text:   "failed",
			Stream: buildlog.StreamMeta,
		})
		return
	}

	//write makefile
	err = writeMakefile(ctx, br.Pkgen, work)
	if err != nil {
		l.Log(buildlog.Line{
			Text:   "makefile generation failed",
			Stream: buildlog.StreamBuild,
		})
		l.Log(buildlog.Line{
			Text:   "failed",
			Stream: buildlog.StreamMeta,
		})
		return
	}

	//send source tarball
	err = writeSourceTar(ctx, br.Pkgen, work, c)
	if err != nil {
		l.Log(buildlog.Line{
			Text:   fmt.Sprintf("source tar generation failed: %q", err.Error()),
			Stream: buildlog.StreamBuild,
		})
		l.Log(buildlog.Line{
			Text:   "failed",
			Stream: buildlog.StreamMeta,
		})
		return
	}

	//run build
	err = work.RunCmd(
		ctx,
		[]string{"make", "-C", "/root/build", "pkgs.tar"},
		nil,
		worker.CmdOptions{LogOut: l},
	)
	if err != nil {
		l.Log(buildlog.Line{
			Text:   fmt.Sprintf("build failed: %q", err.Error()),
			Stream: buildlog.StreamBuild,
		})
		l.Log(buildlog.Line{
			Text:   "failed",
			Stream: buildlog.StreamMeta,
		})
		return
	}

	//send packages back
	pw, err := c.NextWriter(websocket.BinaryMessage)
	if err != nil {
		log.Printf("Oh shoot, failed to send packages back: %q\n", err.Error())
		return
	}
	err = work.ReadFile(ctx, "/root/build/pkgs.tar", pw)
	cerr := pw.Close()
	if cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		log.Printf("Oh shoot, failed to send packages back: %q\n", err.Error())
		l.Log(buildlog.Line{
			Text:   fmt.Sprintf("tar writeback error: %q", err.Error()),
			Stream: buildlog.StreamBuild,
		})
		l.Log(buildlog.Line{
			Text:   "failed",
			Stream: buildlog.StreamMeta,
		})
		return
	}

	l.Log(buildlog.Line{
		Text:   "Done!",
		Stream: buildlog.StreamBuild,
	})
	l.Log(buildlog.Line{
		Text:   "success",
		Stream: buildlog.StreamMeta,
	})
}

// writeSourceTar copies the source tar from the client connection to the worker.
func writeSourceTar(ctx context.Context, pk *pkgen.PackageGenerator, work *worker.Worker, c *websocket.Conn) error {
	_, r, err := c.NextReader()
	if err != nil {
		return err
	}
	return work.WriteFile(ctx, "/root/build/src.tar", r)
}

// writeMakefile generates the Makefile and saves it onto the worker.
func writeMakefile(ctx context.Context, pk *pkgen.PackageGenerator, work *worker.Worker) error {
	err := work.Mkdir(ctx, "/root/build", false)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(nil)
	_, err = pk.GenFullMakefile(pkgen.DefaultVars).WriteTo(buf)
	if err != nil {
		return err
	}
	err = work.WriteFile(ctx, "/root/build/Makefile", buf)
	if err != nil {
		return err
	}
	return nil
}

// doBootstrap runs the bootstrap.sh script on the worker which builds the rootfs.
func doBootstrap(ctx context.Context, c *websocket.Conn, work *worker.Worker, l buildlog.Handler) error {
	mt, r, err := c.NextReader()
	if err != nil {
		return err
	}
	if mt != websocket.BinaryMessage {
		return errors.New("bad message")
	}
	err = work.WriteFile(ctx, "/root/pkgs.tar", r)
	if err != nil {
		return err
	}
	err = work.RunCmd(
		ctx,
		[]string{"/usr/bin/busybox", "sh", "/root/bootstrap.sh"},
		nil,
		worker.CmdOptions{LogOut: l},
	)
	if err != nil {
		return err
	}
	return nil
}

// readWSReq reads a request from the websocket and authenticates it.
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

// errAccessDenied is an error indicating that the authentication was not allowed.
var errAccessDenied = errors.New("access denied")

// authReq decodes a request and checks the authentication validity.
func authReq(raw string, reqsub interface{}) (*internal.Request, error) {
	req, err := internal.DecodeRequest(raw, reqsub)
	if err != nil {
		return nil, err
	}
	for _, v := range auth {
		if bytes.Equal(req.PublicKey, v) {
			return req, nil
		}
	}
	return nil, errAccessDenied
}

// wsLogHandler is a buildlog.Handler which sends the log over a websocket with JSON.
type wsLogHandler struct {
	c *websocket.Conn
}

func (wsl *wsLogHandler) Log(ll buildlog.Line) error {
	return wsl.c.WriteJSON(ll)
}

func (wsl *wsLogHandler) Close() error {
	return nil
}
