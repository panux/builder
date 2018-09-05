package build

import (
	"context"
	"path/filepath"

	"gitlab.com/jadr2ddude/xgraph"
	"gitlab.com/panux/builder/pkgen"
	"gitlab.com/panux/builder/pkgen/buildlog"
	"golang.org/x/tools/godoc/vfs"
)

// GraphOptions is a set of options for a build graph.
type GraphOptions struct {
	// Options contains the base options for the graph.
	Options

	// HashCache is the HashCache used for the build.
	// Optional - will be populated if left empty.
	HashCache *HashCache

	// Cache is the BuildCache to use to implement incremental builds.
	Cache BuildCache

	// Logger is a logging mechanism to use for build jobs.
	Logger buildlog.Logger

	// Arch is the arch to build on.
	Arch pkgen.Arch

	// SourceTree is a vfs used to store the rootfs.
	SourceTree vfs.FileSystem

	rpi RawPackageIndex
}

// job is a build job.
// It implements xgraph.Job.
type job struct {
	info   Info
	loader pkgen.Loader
	pkg    *pkgen.PackageGenerator
	gopts  *GraphOptions
}

func (j *job) Name() string {
	return j.info.PackageName + ":" + j.info.Arch.String()
}

func (j *job) ShouldRun() (bool, error) {
	hash, err := HashPackage(context.Background(), j.pkg, j.loader, j.gopts.HashCache, j.gopts.DockerImage, j.gopts.Dependencies)
	if err != nil {
		return false, err
	}
	j.info.Hash = hash
	return j.gopts.Cache.Valid(j.info)
}

func (j *job) Dependencies() ([]string, error) {
	deps, err := BuildDepsDocker(j.pkg, j.gopts.Dependencies, j.gopts.DockerImage)
	if err != nil {
		return nil, err
	}
	return mapRuleDeps(j.gopts.rpi, j.gopts.Arch, deps...), nil
}

func (j *job) Run(ctx context.Context) error {
	opts := j.gopts.Options
	opts.Ctx = ctx
	log, err := j.gopts.Logger.NewLog(j.Name())
	if err != nil {
		return err
	}
	opts.Log = log
	return Build(j.pkg, opts)
}

func newJob(pkg *RawPkent, opts *GraphOptions) (*job, error) {
	// get subfs
	ns := vfs.NewNameSpace()
	ns.Bind("/", opts.SourceTree, filepath.Dir(pkg.Path), vfs.BindReplace)

	// create loader
	loader, err := pkgen.MultiLoader(opts.Loader, pkgen.FileLoader(ns))
	if err != nil {
		return nil, err
	}

	// preprocess pkgen
	p, err := pkg.Pkgen.Preprocess(opts.Arch, opts.Arch, false)
	if err != nil {
		return nil, err
	}

	// create job
	return &job{
		info: Info{
			PackageName: filepath.Base(filepath.Dir(pkg.Path)),
			Arch:        opts.Arch,
		},
		loader: loader,
		pkg:    p,
		gopts:  opts,
	}, nil
}

// Graph creates a *xgraph.Graph for mass-building packages.
// A meta-rule called "all" is created, which depends on all package rules.
func Graph(rpi RawPackageIndex, opts GraphOptions) (*xgraph.Graph, error) {
	// fix graph options
	if opts.HashCache == nil {
		opts.HashCache = &HashCache{
			m:  map[hashCacheKey]*hashCacheEntry{},
			pr: opts.Packages,
		}
	}

	// create graph
	g := xgraph.New()

	// find pkgens
	lst := rpi.List()
	rules := []string{}
	for _, name := range lst {
		ent := rpi[name]
		if ent.Pkgen.Arch.Supports(opts.Arch) {
			// create job
			job, err := newJob(ent, &opts)
			if err != nil {
				return nil, err
			}
			rules = append(rules, job.Name())
			g.AddJob(job)
		}
	}

	// add "all" meta-rule
	g.AddJob(xgraph.BasicJob{
		JobName: "all",
		Deps:    rules,
		RunCallback: func() error {
			return nil
		},
	})

	return g, nil
}
