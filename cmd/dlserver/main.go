package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/panux/builder/pkgen"
	"github.com/panux/builder/pkgen/dlapi"
)

func main() {
	stopchan := make(chan os.Signal, 1)
	signal.Notify(stopchan, syscall.SIGTERM)
	go func() {
		sig := <-stopchan
		log.Printf("Recieved signal %q, exiting. . . \n", sig.String())
		os.Exit(0)
	}()
	var maxbuf uint
	var h string
	var cachedir string
	flag.UintVar(&maxbuf, "maxbuf", 100*1024*1024, "maximum length of data to buffer in bytes")
	flag.StringVar(&h, "http", ":80", "http address to listen on")
	flag.StringVar(&cachedir, "cache", "/srv/cache/dl", "the directory to store cached downloads in")
	flag.Parse()
	{
		cdinfo, err := os.Stat(cachedir)
		if err != nil {
			log.Fatalf("Failed to stat %q: %q\n", cachedir, err.Error())
		}
		if !cdinfo.IsDir() {
			log.Fatalf("Cache directory %q is not a directory\n", err.Error())
		}
	}
	loader, err := pkgen.NewMultiLoader(
		pkgen.NewHTTPLoader(http.DefaultClient, maxbuf),
	)
	if err != nil {
		log.Fatalf("Failed to create loader: %q\n", err.Error())
	}
	//pre-encode list of supported protocols
	protos, err := loader.SupportedProtocols()
	if err != nil {
		log.Fatalf("Failed to list supported protocols: %q\n", err.Error())
	}
	protodat, err := json.Marshal(protos)
	if err != nil {
		log.Fatalf("Failed to encode supported protocols list: %q\n", err.Error())
	}
	http.HandleFunc("/protos", func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, bytes.NewReader(protodat))
	})
	//handle status requests
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(&dlapi.Status{
			Status:  "running",
			Version: "0.1",
		})
	})
	//handle download requests
	http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			log.Printf("Failed to parse form: %q\n", err.Error())
			return
		}
		ustr := r.FormValue("url")
		if ustr == "" {
			http.Error(w, "missing url in query", http.StatusBadRequest)
			return
		}
		u, err := url.Parse(ustr)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to parse url: %q", err.Error()), http.StatusBadRequest)
			log.Printf("Failed to parse url: %q\n", err.Error())
			return
		}
		for _, p := range protos {
			if u.Scheme == p {
				goto itworks
			}
		}
		if ustr == "" {
			http.Error(w, pkgen.ErrUnsupportedProtocol.Error(), http.StatusBadRequest)
			return
		}
	itworks:
		//generate file path
		fp := filepath.Join(
			cachedir,
			filepath.Clean(
				strings.Replace(u.String(), "/", "_s", -1),
			),
		)
		//open file
		f, err := os.OpenFile(fp, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			http.Error(w, "failed to open file", http.StatusInternalServerError)
			log.Printf("Failed to open file: %q\n", err.Error())
			return
		}
		defer f.Close()
		inf, err := f.Stat()
		if err != nil {
			http.Error(w, "failed to stat file", http.StatusInternalServerError)
			log.Printf("Failed to stat file: %q\n", err.Error())
			return
		}
		if inf.Size() == 0 { //it is not cached - load it
			var donesaving bool
			defer func() {
				if !donesaving { //delete file if incomplete write
					os.Remove(fp)
				}
			}()
			//download
			_, r, err := loader.Get(context.Background(), u)
		dlfail:
			if err != nil {
				http.Error(w, "download failure", http.StatusInternalServerError)
				log.Printf("Failed to download %q: %q\n", u.String(), err.Error())
				return
			}
			_, err = io.Copy(f, r)
			if err != nil {
				goto dlfail
			}
			//seek to beginning
			_, err = f.Seek(0, 0)
			if err != nil {
				http.Error(w, "internal failure", http.StatusInternalServerError)
				log.Printf("Failed to seek: %q\n", err.Error())
				return
			}
			err = f.Sync()
			if err != nil {
				http.Error(w, "internal failure", http.StatusInternalServerError)
				log.Printf("Failed to sync: %q\n", err.Error())
				return
			}
			donesaving = true
		}
		io.Copy(w, f) //write it out
	})
	//start server
	errch := make(chan error)
	go func() {
		errch <- http.ListenAndServe(h, nil)
	}()
	log.Printf("Started http server on %q\n", h)
	log.Fatalf("Failed: %q\n", (<-errch).Error())
}
