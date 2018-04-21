package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/panux/builder/internal"
	"github.com/panux/builder/pkgen"
	"github.com/panux/builder/pkgen/buildlog"
	"github.com/panux/builder/pkgen/dlapi"
	"github.com/panux/builder/pkgen/worker"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var starter *worker.Starter
var auth [][]byte
var loader pkgen.Loader

func main() {
	var addr string
	var namespace string
	var authkeys string
	var dlserver string
	flag.StringVar(&addr, "http", ":80", "http listen address")
	flag.StringVar(&namespace, "namespace", "default", "Kubernetes namespace to run workers in")
	flag.StringVar(&authkeys, "auth", "/srv/authkeys.json", "JSON file containing authorized RSA auth keys")
	flag.StringVar(&dlserver, "dlserver", "http://dlserver/", "address of download server")
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

	//Prep loader
	dlurl, err := url.Parse(dlserver)
	if err != nil {
		log.Fatalf("Failed to parse dlserver URL: %q\n", err.Error())
	}
	loader = dlapi.NewDlClient(dlurl, nil)

	//http
	http.HandleFunc("/build", handleBuild)
	log.Fatalf("failed to listen and serve: %q\n", http.ListenAndServe(addr, nil))
}

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

var wsup = &websocket.Upgrader{ //websocket upgrader
	HandshakeTimeout: time.Second * 30,
}

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
	req, err := readWSReq(c, internal.BuildRequest{})
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
	br := req.Request.(internal.BuildRequest)

	//start worker
	work, err := starter.Start(br.Pkgen)
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
				append(
					[]string{"apk", "--no-cache", "add"},
					br.Pkgen.BuildDependencies...,
				),
				nil,
				worker.CmdOptions{LogOut: l},
			)
		}
	} else {
		err = doBootstrap(c, work, l)
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
	err = writeMakefile(br.Pkgen, work)
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
	err = writeSourceTar(br.Pkgen, work)
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
	err = work.ReadFile("/root/build/pkgs.tar", pw)
	pw.Close()
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

func writeSourceTar(pk *pkgen.PackageGenerator, work *worker.Worker) error {
	piper, pipew := io.Pipe()
	go func() { //write source tar on a seperate goroutine
		pipew.CloseWithError(pk.WriteSourceTar(pipew, loader, 100*1024*1024))
	}()
	defer piper.Close() //insurance that the background goroutine wont survive
	return work.WriteFile("/root/build/src.tar", piper)
}

func writeMakefile(pk *pkgen.PackageGenerator, work *worker.Worker) error {
	err := work.Mkdir("/root/build", false)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(nil)
	_, err = pk.GenFullMakefile(pkgen.DefaultVars).WriteTo(buf)
	if err != nil {
		return err
	}
	err = work.WriteFile("/root/build/Makefile", buf)
	if err != nil {
		return err
	}
	return nil
}

func doBootstrap(c *websocket.Conn, work *worker.Worker, l buildlog.Handler) error {
	mt, r, err := c.NextReader()
	if err != nil {
		return err
	}
	if mt != websocket.BinaryMessage {
		return errors.New("bad message")
	}
	err = work.WriteFile("/root/pkgs.tar", r)
	if err != nil {
		return err
	}
	err = work.RunCmd(
		[]string{"/usr/bin/busybox", "sh", "/root/bootstrap.sh"},
		nil,
		worker.CmdOptions{LogOut: l},
	)
	if err != nil {
		return err
	}
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

var errAccessDenied = errors.New("access denied")

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

type wsLogHandler struct {
	c *websocket.Conn
}

func (wsl *wsLogHandler) Log(ll buildlog.Line) error {
	return wsl.c.WriteJSON(ll)
}

func (wsl *wsLogHandler) Close() error {
	return nil
}
