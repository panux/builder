package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"gitlab.com/jadr2ddude/xgraph"
	"gitlab.com/panux/builder/pkgen"
	"gitlab.com/panux/builder/pkgen/buildmanager"
	"golang.org/x/tools/godoc/vfs"
)

func main() {
	defer log.Println("Shutdown complete")

	//load config
	var configfile string
	flag.StringVar(&configfile, "config", "conf.json", "JSON format config file")
	flag.Parse()
	loadConfig(configfile)

	//set up sync.WaitGroup for shutdown
	var wg sync.WaitGroup
	defer wg.Wait()

	//Set up server-wide context with SIGTERM cancellation
	srvctx, srvcancel := context.WithCancel(context.Background())
	sigch := make(chan os.Signal, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-sigch
		log.Println("Initiating shutdown")
		srvcancel()
	}()
	signal.Notify(sigch, syscall.SIGTERM)

	//Set up logging
	logmanager := &LogManager{
		store: &LogStore{
			path: Config.LogDir,
		},
		buildlookup: make(map[[sha256.Size]byte]*LogSession),
	}

	//Set up build manager client
	bmcli := setupBMClient()

	//Set up build cache
	bcache := buildmanager.NewJSONDirCache(Config.CacheDir)

	//Set up package storage
	pkstore := &PackageStore{
		dir: Config.OutputDir,
	}

	//Set up repo
	repo, err := NewGitRepo(srvctx, Config.GitRepo, Config.GitDir)
	if err != nil {
		log.Fatalf("failed to git clone: %q", err.Error())
	}

	//Set up loader
	baseloader := pkgen.NewHTTPLoader(http.DefaultClient, Config.MaxBuf)

	//Set up work pool
	workpool := xgraph.NewWorkPool(Config.Parallel)
	defer workpool.Close()

	//branch state
	branch := &BranchStatus{BranchName: "beta"}

	//do build loop
	wg.Add(1)
	startch := make(chan struct{})
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(time.Minute * 15)
		srvstop := srvctx.Done()
		defer ticker.Stop()
		for {
			rb := func() {
				log.Println("Starting build. . .")
				err := repo.WithBranch(srvctx, "beta", func(ctx context.Context, source vfs.FileSystem) error {
					return (&buildmanager.Builder{
						LogProvider:      logmanager,
						Client:           bmcli,
						BuildCache:       bcache,
						Output:           pkstore,
						SourceTree:       source,
						PackageRetriever: pkstore,
						BaseLoader:       baseloader,
						Arch:             Config.Arch,
						WorkRunner:       workpool,
						EventHandler:     branch,
						InfoCallback:     branch.infoCallback,
					}).Build(ctx, branch.ListCallback)
				})
				if err != nil {
					fmt.Printf("Failed to build: %q\n", err.Error())
				}
				cmd := exec.CommandContext(srvctx, Config.AfterBuild[0], Config.AfterBuild[1:]...)
				cmd.Stderr = os.Stderr
				cmd.Stdout = os.Stdout
				err = cmd.Run()
				if err != nil {
					fmt.Printf("Failed to run post-build command: %q\n", err.Error())
				}
			}
			select {
			case <-ticker.C:
				rb()
			case <-startch:
				rb()
			case <-srvstop:
				return
			}
		}
	}()

	go func() {
		startch <- struct{}{}
	}()

	//configure HTTP router
	router := http.DefaultServeMux
	router.Handle("/api/branch", branch)
	router.Handle("/api/log", logmanager)
	router.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) { //status probe for Kubernetes
		err := bmcli.Status()
		if err != nil {
			http.Error(w, fmt.Sprintf("build manager status error: %q", err.Error()), http.StatusInternalServerError)
			log.Printf("failed status probe: %q\n", err.Error())
			return
		}
		w.Write([]byte("online"))
	})
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "public")
		w.Header().Add("Cache-Control", "max-age=60")
		w.Header().Add("Cache-Control", "stale-if-error=120")
		http.FileServer(http.Dir(Config.Static)).ServeHTTP(w, r)
	})
	router.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		go func() { startch <- struct{}{} }()
		w.Write([]byte("ok"))
	})

	//start HTTP server
	server := &http.Server{
		Addr:    Config.HTTPAddr,
		Handler: router,
	}
	wg.Add(1)
	go func() { //run server in goroutine
		defer wg.Done()
		log.Println("starting web server. . . ")
		err := server.ListenAndServe()
		if err != nil {
			log.Printf("Web server failed: %q\n", err.Error())
			close(sigch) //trigger shutdown
		}
	}()
	wg.Add(1)
	go func() { //trigger http shutdown on srvctx cancel
		defer wg.Done()
		<-srvctx.Done()
		err := server.Shutdown(context.Background())
		if err != nil {
			log.Printf("shutdown error: %q\n", err.Error())
		}
	}()

	//wait for cancellation
	<-srvctx.Done()
}

// loadConfig reads the Config from the file at the given path.
func loadConfig(path string) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("failed to load config: %q", err.Error())
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&Config)
	if err != nil {
		log.Fatalf("failed to load config: %q", err.Error())
	}
}

// setupBMClient sets up a build manager client
func setupBMClient() *buildmanager.Client {
	bmurl, err := url.Parse(Config.BuildManager)
	if err != nil {
		log.Fatalf("failed to parse build manager URL: %q", err.Error())
	}
	keydat, err := ioutil.ReadFile(Config.BuildManagerKey)
	if err != nil {
		log.Fatalf("failed to load build manager key file: %q", err.Error())
	}
	pkey, err := x509.ParsePKCS1PrivateKey(keydat)
	if err != nil {
		log.Fatalf("failed to parse build manager key: %q", err.Error())
	}
	return buildmanager.NewClient(bmurl, pkey, websocket.DefaultDialer)
}

// Config is the configuration struct
var Config struct {
	HTTPAddr        string        `json:"http"`
	Branches        []string      `json:"branches"`
	Static          string        `json:"static"`
	LogDir          string        `json:"logs"`
	BuildManager    string        `json:"manager"`
	BuildManagerKey string        `json:"managerKey"`
	CacheDir        string        `json:"cache"`
	OutputDir       string        `json:"output"`
	GitDir          string        `json:"gitDir"`
	GitRepo         string        `json:"gitRepo"`
	Arch            pkgen.ArchSet `json:"arch"`
	Parallel        uint16        `json:"parallel"`
	MaxBuf          uint          `json:"maxbuf"`
	AfterBuild      []string      `json:"afterBuild"`
}
