package buildmanager

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gitlab.com/jadr2ddude/xgraph"
	"gitlab.com/panux/builder/pkgen"
	"golang.org/x/tools/godoc/vfs"
)

// HashCache is a cache of hashes
type HashCache struct {
	m    map[hashCacheKey]*hashCacheEntry
	pr   PackageRetriever
	scan uint64
}

// Prune removes hash cache entries that have not been used recently.
func (hc *HashCache) Prune() {
	for k, v := range hc.m {
		if v.scan != hc.scan {
			delete(hc.m, k)
		}
	}
}

type hashCacheKey struct {
	name      string
	arch      pkgen.Arch
	bootstrap bool
}

type hashCacheEntry struct {
	hash      [sha256.Size]byte
	scan      uint64
	timestamp time.Time
}

func (hc *HashCache) hash(name string, arch pkgen.Arch, bootstrap bool) (hash [sha256.Size]byte, err error) {
	hck := hashCacheKey{
		name:      name,
		arch:      arch,
		bootstrap: bootstrap,
	}

	//lookup in cache
	hce := hc.m[hck]
	if hce != nil && hce.scan == hc.scan {
		return hce.hash, nil
	}
	if hce == nil {
		hce = new(hashCacheEntry)
		hc.m[hck] = hce
	}
	hce.scan = hc.scan

	defer func() {
		//flush cache entry on error
		if err != nil {
			delete(hc.m, hck)
		}
	}()

	//get package
	_, r, _, err := hc.pr.GetPkg(name, arch, bootstrap)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer func() {
		cerr := r.Close()
		if cerr != nil && err == nil {
			err = cerr
			hash = [sha256.Size]byte{}
		}
	}()
	//timestamp checking option
	if f, ok := r.(*os.File); ok {
		inf, err := f.Stat()
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		t := inf.ModTime()
		if t.Equal(hce.timestamp) {
			return hce.hash, nil
		}
		hce.timestamp = t
	}

	//hash package
	h := sha256.New()
	_, err = io.Copy(h, r)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	var ha [sha256.Size]byte
	copy(ha[:], h.Sum(nil))

	//store to cache
	hce.hash = ha

	return ha, nil
}

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

	// HashCache is a hash cache for packages
	HashCache *HashCache
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
		buider:       b,
		pkgname:      name,
		pk:           pk,
		bootstrapped: pkgen.Builder(ent.Pkgen.Builder).IsBootstrap(),
		err:          err,
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
				deps, _ := bj.Dependencies()
				if deps != nil {
					log.Printf("Build %q depends on %v\n", bj.Name(), deps)
				}
				if pkgen.Builder(pke.Pkgen.Builder).IsBootstrap() {
					bj = b.genBuildJob(pke, arch, true)
					g.AddJob(bj)
					things = append(things, bj.Name())
					deps, _ = bj.Dependencies()
					if deps != nil {
						log.Printf("Build %q depends on %v\n", bj.Name(), deps)
					}
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

// GetGraph gets a build graph and a list of jobs.
func (b *Builder) GetGraph() (*xgraph.Graph, []string, error) {
	b.HashCache = &HashCache{
		m:  make(map[hashCacheKey]*hashCacheEntry),
		pr: b.PackageRetriever,
	}
	err := b.prepRPG()
	if err != nil {
		return nil, nil, err
	}
	return b.genGraph()
}

// Build runs a build using the given builder.
// Before starting the build, lcb is called with the list of targets.
// The provided context supports cancellation.
func (b *Builder) Build(ctx context.Context, listcallback func([]string) error) error {
	if b.HashCache == nil {
		b.HashCache = &HashCache{
			m:  make(map[hashCacheKey]*hashCacheEntry),
			pr: b.PackageRetriever,
		}
	} else {
		b.HashCache.scan++
	}
	defer b.HashCache.Prune()
	err := b.prepRPG()
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

// parseJobName parses the name of a job into identifiers for a build.
func parseJobName(jobname string) (name string, arch pkgen.Arch, bootstrap bool) {
	if strings.HasSuffix(jobname, "-bootstrap") {
		bootstrap = true
		jobname = strings.TrimSuffix(jobname, "-bootstrap")
	}
	spl := strings.Split(jobname, ":")
	if len(spl) < 2 {
		return "fail", "", false
	}
	name = spl[0]
	arch = pkgen.Arch(spl[1])
	return name, arch, bootstrap
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
	if bj.pk.Builder.IsBootstrap() {
		return []string{}, nil
	}
	pkfs, err := bj.buider.index.DepWalker().
		Walk(append(bj.pk.BuildDependencies, "build-meta")...)
	if err != nil {
		return nil, err
	}
	pkfs = dedup(pkfs)
	for i, v := range pkfs {
		bld := bj.buider.index[v]
		pkfs[i] += ":" + bj.pk.HostArch.String()
		if pkgen.Builder(bld.Pkgen.Builder).IsBootstrap() {
			pkfs[i] += "-bootstrap"
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
	if !bj.pk.Builder.IsBootstrap() {
		pkfs, err := bj.Dependencies()
		if err != nil {
			return nil, err
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
			h, err := bj.buider.HashCache.hash(parseJobName(v))
			ent.Hash = h[:]
			ent.Name += ".tar"
			return
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
	il, err := bj.buider.BuildCache.CheckLatest(bi)
	if il {
		if err == nil {
			log.Printf("Caching build %q\n", bj.Name())
		} else {
			log.Printf("Caching build %q with failure\n", bj.Name())
		}
	}
	return !il, err
}

func (bj *buildJob) Dependencies() ([]string, error) {
	if bj.pk == nil {
		return []string{}, bj.err
	}
	if bj.pk.Builder.IsBootstrap() {
		//no deps
		return []string{}, nil
	}
	pkfs, err := bj.pkgDeps()
	if err != nil {
		return nil, err
	}
	for i, v := range pkfs {
		parts := strings.Split(v, ":")
		bld := bj.buider.index[parts[0]]
		pkfs[i] = filepath.Base(filepath.Dir(bld.Path)) + ":" + parts[1]
	}
	pkfs = dedup(pkfs)
	return pkfs, nil
}

func dedup(in []string) []string {
	m := make(map[string]struct{})
	for _, v := range in {
		m[v] = struct{}{}
	}
	o := make([]string, len(m))
	i := 0
	for v := range m {
		o[i] = v
		i++
	}
	sort.Strings(o)
	return o
}

func (bj *buildJob) Run(ctx context.Context) (err error) {
	//get build info
	bi, err := bj.buildInfo()
	if err != nil {
		return err
	}

	//set up loader
	vns := vfs.NewNameSpace()
	vns.Bind("/", bj.buider.SourceTree, filepath.Dir(bj.buider.index[bj.pkgname].Path), vfs.BindReplace)
	load, err := pkgen.NewMultiLoader(pkgen.NewFileLoader(vns), bj.buider.BaseLoader)
	if err != nil {
		return err
	}

	//create the BuildJobRequest
	bdeps, err := bj.pkgDeps()
	if err != nil {
		return err
	}
	bjr, err := bj.buider.CreateBuildJobRequest(bj.pk, bdeps, bj.buider.PackageRetriever, load)
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
	err = bj.buider.Client.Build(bjr, BuildOptions{
		Out: func(name string, r io.Reader) error {
			return bj.buider.Output.Store(inf, name, ioutil.NopCloser(r))
		},
		LogOut: log,
	})
	if err != nil && err.Error() != "failed" {
		return err
	}

	var bcerror string
	if err != nil {
		bcerror = err.Error()
	}
	cerr := bj.buider.BuildCache.UpdateCache(BuildCacheEntry{
		BuildInfo: bi,
		Error:     bcerror,
	})
	if cerr != nil {
		return cerr
	}
	if err != nil {
		return err
	}

	return nil
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
