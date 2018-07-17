package main

import (
	"crypto/x509"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/gorilla/websocket"
	"gitlab.com/jadr2ddude/xgraph"
	"gitlab.com/panux/builder/internal/srvctx"
	"gitlab.com/panux/builder/pkgen"
	"gitlab.com/panux/builder/pkgen/buildlog"
	"gitlab.com/panux/builder/pkgen/buildmanager"
	"golang.org/x/tools/godoc/vfs"
)

// source dir
var srcDir string

// buildManager URL
var buildManager string

// buildManager key path
var buildManagerKey string

// cache directory
var cacheDir string

// output directory path
var outDir string

// number of parallel builds
var parallel uint

// maximum buffering size
var maxBuf uint

var failed bool

type evh struct{}

func (e evh) OnQueued(job string) {
	log.Printf("Queued job %q. . .", job)
}

func (e evh) OnStart(job string) {
	log.Printf("Starting job %q. . .", job)
}

func (e evh) OnFinish(job string) {
	log.Printf("Completed job %q.", job)
}

func (e evh) OnError(job string, err error) {
	log.Printf("Job %q failed: %q", job, err)
	failed = true
}

type lh struct {
	bi buildmanager.BuildInfo
}

func (l lh) Log(ln buildlog.Line) error {
	var bs string
	if l.bi.Bootstrap {
		bs = "-bootstrap"
	}
	log.Printf("[%s%s%s]%s", l.bi.PackageName, l.bi.Arch.String(), bs, ln.Text)
	return nil
}

func (l lh) Close() error {
	return nil
}

type lp struct{}

func (l lp) Log(bi buildmanager.BuildInfo) (buildlog.Handler, error) {
	return lh{bi: bi}, nil
}

func main() {
	defer func() {
		if failed {
			os.Exit(65)
		}
	}()

	// graceful shutdown
	defer srvctx.Wait.Wait()

	// parse flags
	flag.StringVar(&srcDir, "src", ".", "package source directory")
	flag.StringVar(&buildManager, "bmurl", "", "build manager URL")
	flag.StringVar(&buildManager, "bmkey", "", "build manager key")
	flag.StringVar(&cacheDir, "cache", "./cache", "cache directory")
	flag.StringVar(&outDir, "outdir", "./out", "output directory")
	flag.UintVar(&parallel, "parallel", 2, "number of builds to run simultaneously")
	flag.UintVar(&maxBuf, "buf", 1<<24, "maximum source buffering length")
	flag.Parse()

	// connect to build manager
	bmurl, err := url.Parse(buildManager)
	if err != nil {
		log.Fatalf("failed to parse build manager URL: %s", err.Error())
	}
	keydat, err := ioutil.ReadFile(buildManagerKey)
	if err != nil {
		log.Fatalf("failed to load build manager key: %s", err.Error())
	}
	pkey, err := x509.ParsePKCS1PrivateKey(keydat)
	if err != nil {
		log.Fatalf("failed to parse build manager key: %s", err.Error())
	}
	bmcli := buildmanager.NewClient(bmurl, pkey, websocket.DefaultDialer)
	err = bmcli.Status()
	if err != nil {
		log.Fatalf("Failed to connect to build manager: %s", err.Error())
	}

	// set up build cache
	bcache := buildmanager.NewJSONDirCache(cacheDir)

	// set up package storage
	pkstore := &PackageStore{
		dir: outDir,
	}

	// set up loader
	baseloader := pkgen.NewHTTPLoader(http.DefaultClient, maxBuf)

	// set up work pool
	workpool := xgraph.NewWorkPool(uint16(parallel))
	defer workpool.Close()

	// cancel server context before terminating
	defer srvctx.Cancel()

	// create builder
	bldr := &buildmanager.Builder{
		LogProvider:      lp{},
		Client:           bmcli,
		BuildCache:       bcache,
		Output:           pkstore,
		SourceTree:       vfs.OS(srcDir),
		PackageRetriever: pkstore,
		BaseLoader:       baseloader,
		Arch:             pkgen.SupportedArch,
		WorkRunner:       workpool,
		EventHandler:     evh{},
	}

	// execute build
	bldr.Build(srvctx.Context, nil, flag.Args()...)
}
