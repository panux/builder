package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"

	"github.com/docker/docker/client"
	"gitlab.com/jadr2ddude/xgraph"
	"gitlab.com/panux/builder/pkgen"
	"gitlab.com/panux/builder/pkgen/build"
	"gitlab.com/panux/builder/pkgen/buildlog"
	"golang.org/x/tools/godoc/vfs"
)

func main() {
	dcli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}
	defer dcli.Close()
	stree := vfs.OS(".")
	rpi, err := build.IndexDir(stree)
	if err != nil {
		panic(err)
	}
	wp := xgraph.NewWorkPool(4)
	defer wp.Close()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		cancel()
	}()
	arch, err := pkgen.GetHostArch()
	if err != nil {
		panic(err)
	}
	dirstore := build.DirStore("out")
	logger := buildlog.TextLogger(os.Stderr)
	g, err := build.Graph(rpi, build.GraphOptions{
		Options: build.Options{
			Docker:       dcli,
			DockerImage:  loadDockerImg(rpi),
			Output:       dirstore,
			Packages:     dirstore,
			Dependencies: rpi,
			Loader: pkgen.BufferLoader(
				pkgen.HTTPLoader(
					nil,
					10*1024*1024,
				),
				10*1024*1024),
			Ctx: ctx,
		},
		Cache:      build.DirJSONCache("cache"),
		Logger:     logger,
		Arch:       arch,
		SourceTree: stree,
	})
	if err != nil {
		panic(err)
	}
	ehlog, err := logger.NewLog("status")
	if err != nil {
		panic(err)
	}
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"all"}
	}
	(&xgraph.Runner{
		Graph:      g,
		WorkRunner: wp,
		EventHandler: logEVH{
			l: ehlog,
		},
	}).Run(ctx, args...)
}

func loadDockerImg(rpi build.RawPackageIndex) build.Image {
	var img build.Image
	f, err := os.Open("docker.json")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&img)
	if err != nil {
		panic(err)
	}
	pkgs, err := rpi.FindDependencies(img.Packages...)
	if err != nil {
		panic(err)
	}
	img.Packages = pkgs
	return img
}

type logEVH struct {
	l buildlog.Handler
}

func (le logEVH) OnQueued(job string) {
	le.l.Log(buildlog.Line{
		Stream: buildlog.StreamMeta,
		Text:   fmt.Sprintf("job %q queued", job),
	})
}
func (le logEVH) OnStart(job string) {
	le.l.Log(buildlog.Line{
		Stream: buildlog.StreamMeta,
		Text:   fmt.Sprintf("job %q started", job),
	})
}
func (le logEVH) OnFinish(job string) {
	le.l.Log(buildlog.Line{
		Stream: buildlog.StreamMeta,
		Text:   fmt.Sprintf("job %q finished", job),
	})
}
func (le logEVH) OnError(job string, err error) {
	le.l.Log(buildlog.Line{
		Stream: buildlog.StreamMeta,
		Text:   fmt.Sprintf("job %q errored: %s", job, err.Error()),
	})
}
