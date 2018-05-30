package buildmanager

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/jadr2ddude/xgraph"
	"github.com/panux/builder/pkgen"
	"golang.org/x/tools/godoc/vfs"
)

// Builder is a tool to build packages. All fields are required.
type Builder struct {

	// LogProvider is the source of LogHandlers used for builds.
	LogProvider LogProvider

	// Client is the buildmanager Client used.
	Client *Client

	// BuildCache is the BuildCache used to track incremental builds.
	BuildCache BuildCache

	// Output is the OutputHandler used to store the generated packages.
	Output OutputHandler

	// SourceTree is a vfs used to store the rootfs.
	SourceTree vfs.FileSystem

	// index is the RawPackageIndex used.
	index RawPackageIndex

	// PackageRetriever is the PackageRetriever used to load packages used for the build.
	PackageRetriever PackageRetriever

	// BaseLoader is the pkgen.Loader used to load files over the network.
	BaseLoader pkgen.Loader

	// Arch is the set of architectures to build packages for.
	Arch pkgen.ArchSet

	// WorkRunner is the xgraph.WorkRunner to use to run the build.
	WorkRunner xgraph.WorkRunner

	// EventHandler is the xgraph.EventHandler used for the build.
	EventHandler xgraph.EventHandler

	// InfoCallback is a callback run when build info is generated.
	InfoCallback func(jobName string, info BuildInfo) error
}

// genBuildJob creates a *buildJob with the given package entry, targeting the given arch.
// If bootstrap is true, the package will be built as bootstrap.
func (b *Builder) genBuildJob(ent *RawPkent, arch pkgen.Arch, bootstrap bool) *buildJob {
	//get name
	name := filepath.Base(filepath.Dir(ent.Path))

	//preprocess pkgen
	pk, err := ent.Pkgen.Preprocess(arch, arch, bootstrap)
	if err != nil {
		log.Printf("Preprocessing error for %v-%v-%v: %s\n", ent, arch, bootstrap, err.Error())
	}

	return &buildJob{
		buider:  b,
		pkgname: name,
		pk:      pk,
		err:     err,
	}
}

// genGraph uses genBuildJob to build an xgraph of buildJobs.
func (b *Builder) genGraph() (*xgraph.Graph, []string, error) {
	g := xgraph.New()
	things := []string{}
	for _, name := range b.index.List() {
		pke := b.index[name]
		if pke == nil || pke.Pkgen == nil {
			continue
		}
		for _, arch := range b.Arch {
			if pke.Pkgen.Arch.Supports(arch) {
				bj := b.genBuildJob(pke, arch, false)
				g.AddJob(bj)
				things = append(things, bj.Name())
				if pkgen.Builder(pke.Pkgen.Builder).IsBootstrap() {
					bj = b.genBuildJob(pke, arch, true)
					g.AddJob(bj)
					things = append(things, bj.Name())
				}
			}
		}
	}
	g.AddJob(xgraph.BasicJob{
		JobName:     "all",
		Deps:        things,
		RunCallback: func() error { return nil },
	})
	return g, things, nil
}

func (b *Builder) prepRPG() error {
	rpg, err := IndexDir(b.SourceTree)
	if err != nil {
		return err
	}
	b.index = rpg
	return nil
}

// Build runs a build using the given builder.
// Before starting the build, lcb is called with the list of targets.
// The provided context supports cancellation.
func (b *Builder) Build(ctx context.Context, listcallback func([]string) error) error {
	err := b.prepRPG()
	log.Println(spew.Sprint(b))
	if err != nil {
		return err
	}
	g, lst, err := b.genGraph()
	if err != nil {
		return err
	}
	err = listcallback(lst)
	if err != nil {
		return err
	}
	(&xgraph.Runner{
		Graph:        g,
		WorkRunner:   b.WorkRunner,
		EventHandler: b.EventHandler,
	}).Run(ctx, "all")
	return nil
}

// buildJob is an xgraph.Job for a build.
type buildJob struct {
	// builder is the *Builder that this job was created by.
	buider *Builder

	// pkgname is the name of the package being built.
	pkgname string

	//bootstrap indicates whether this is bootstrapped before build.
	bootstrapped bool

	//pk is the *pkgen.PackageGenerator being built.
	pk *pkgen.PackageGenerator

	//err is a preprocessing error
	err error
}

func (bj *buildJob) Name() string {
	if bj.pk == nil {
		return "failed-build-" + strconv.FormatInt(rand.Int63(), 10)
	}
	suffix := ""
	if bj.pk.Builder.IsBootstrap() {
		suffix = "-bootstrap"
	}
	return bj.pkgname + ":" + bj.pk.BuildArch.String() + suffix
}

// pkgDeps gets a list of package rules which are dependencies.
func (bj *buildJob) pkgDeps() ([]string, error) {
	if bj.err != nil {
		return nil, bj.err
	}
	pkfs, err := bj.buider.index.DepWalker().
		Walk(append(bj.pk.BuildDependencies, "build-meta")...)
	if err != nil {
		return nil, err
	}
	sort.Strings(pkfs)
	for i := 1; i < len(pkfs); {
		if pkfs[i] == pkfs[i-1] {
			pkfs = pkfs[:i+copy(pkfs[i:], pkfs[i+1:])]
		} else {
			i++
		}
	}
	if bj.bootstrapped {
		for i, v := range pkfs {
			pkfs[i] = v + "-bootstrap"
		}
	}
	return pkfs, nil
}

// hash gets a hash of all of the inputs of a build.
func (bj *buildJob) hash() ([]byte, error) {
	bleh := []string{}
	for _, v := range bj.pk.Sources {
		if v.Scheme == "file" {
			bleh = append(bleh, filepath.Join(filepath.Dir(bj.buider.index[bj.pkgname].Path), filepath.Clean(v.Path)))
		}
	}
	pkhs := []string{}
	if bj.pk.Builder.IsBootstrap() {
		pkfs, err := bj.pkgDeps()
		if err != nil {
			return nil, err
		}
		for i, v := range pkfs {
			v = strings.TrimSuffix(v, "-bootstrap")
			pkfs[i] = v
		}
		pkhs = pkfs
	}
	blents := make([]struct {
		Name string `json:"name"`
		Hash []byte `json:"hash"`
	}, len(bleh)+len(pkhs)+1)
	for i, v := range bleh {
		blents[i].Name = filepath.Base(v)
		h, err := func() ([]byte, error) {
			f, err := bj.buider.SourceTree.Open(v)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			h := sha256.New()
			_, err = io.Copy(h, f)
			if err != nil {
				return nil, err
			}
			return h.Sum(nil), nil
		}()
		if err != nil {
			return nil, err
		}
		blents[i].Hash = h
	}
	for i, v := range pkhs {
		//read and hash packages used
		ent := &blents[i+len(bleh)]
		err := func() (err error) {
			ent.Name = v
			_, r, ext, err := bj.buider.PackageRetriever.GetPkg(v, bj.pk.BuildArch, bj.bootstrapped)
			if err != nil {
				return err
			}
			defer func() {
				cerr := r.Close()
				if cerr != nil && err == nil {
					err = cerr
				}
			}()
			ent.Name += ".tar." + ext
			h := sha256.New()
			_, err = io.Copy(h, r)
			if err != nil {
				return err
			}
			ent.Hash = h.Sum(nil)
			return nil
		}()
		if err != nil {
			return nil, err
		}
	}
	blents[len(bleh)+len(pkhs)].Name = "pkgen.yaml"
	pkh := sha256.New()
	err := json.NewEncoder(pkh).Encode(bj.pk)
	if err != nil {
		return nil, err
	}
	blents[len(bleh)+len(pkhs)].Hash = pkh.Sum(nil)
	oh := sha256.New()
	err = json.NewEncoder(oh).Encode(blents)
	if err != nil {
		return nil, err
	}
	return oh.Sum(nil), nil
}

// buildInfo returns the BuildInfo for the buildJob.
func (bj *buildJob) buildInfo() (BuildInfo, error) {
	h, err := bj.hash()
	if err != nil {
		return BuildInfo{}, err
	}
	var sh [sha256.Size]byte
	copy(sh[:], h)
	return BuildInfo{
		PackageName: bj.pkgname,
		Arch:        bj.pk.BuildArch,
		Hash:        sh,
		Bootstrap:   bj.pk.Builder.IsBootstrap(),
	}, nil
}

func (bj *buildJob) ShouldRun() (bool, error) {
	bi, err := bj.buildInfo()
	if err != nil {
		return false, err
	}
	err = bj.buider.InfoCallback(bj.Name(), bi)
	if err != nil {
		return false, err
	}
	return bj.buider.BuildCache.CheckLatest(bi)
}

func (bj *buildJob) Dependencies() ([]string, error) {
	if bj.pk.Builder.IsBootstrap() {
		//no deps
		return []string{}, nil
	}
	pkfs, err := bj.buider.index.DepWalker().
		Walk(append(bj.pk.BuildDependencies, "build-meta")...)
	if err != nil {
		return nil, err
	}
	return pkfs, nil
}

func (bj *buildJob) Run(ctx context.Context) (err error) {
	//set up loader
	vns := vfs.NewNameSpace()
	vns.Bind("/", bj.buider.SourceTree, filepath.Dir(bj.buider.index[bj.pkgname].Path), vfs.BindReplace)
	load, err := pkgen.NewMultiLoader(pkgen.NewFileLoader(vns), bj.buider.BaseLoader)
	if err != nil {
		return err
	}

	//create the BuildJobRequest
	bjr, err := CreateBuildJobRequest(bj.pk, bj.buider.index.DepWalker(), bj.buider.PackageRetriever, load)
	if err != nil {
		return err
	}

	//prep logging
	inf, err := bj.buildInfo()
	if err != nil {
		return err
	}
	log, err := bj.buider.LogProvider.Log(inf)
	if err != nil {
		return err
	}
	defer func() {
		cerr := log.Close()
		if cerr != nil && err != nil {
			err = cerr
		}
	}()

	//run build
	return bj.buider.Client.Build(bjr, BuildOptions{
		Out: func(name string, r io.Reader) error {
			return bj.buider.Output.Store(inf, name, ioutil.NopCloser(r))
		},
		LogOut: log,
	})
}

// OutputHandler is an interface to handle the output of builds.
type OutputHandler interface {
	Store(build BuildInfo, filename string, body io.ReadCloser) error
}

// PackageRetriever is an interface to load packages.
type PackageRetriever interface {
	// GetPkg gets a package with the given name and arch.
	GetPkg(name string, arch pkgen.Arch, bootstrap bool) (len uint32, r io.ReadCloser, ext string, err error)
}
