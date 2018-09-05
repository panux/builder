package build

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"gitlab.com/panux/builder/pkgen"
	"gitlab.com/panux/builder/pkgen/buildlog"
)

// DependencyFinder is an interface used to find dependencies of packages recursively.
// Implemented by RawPackageIndex.
type DependencyFinder interface {
	FindDependencies(...string) ([]string, error)
}

// Image is a container image usable for building.
type Image struct {
	// Image is a docker image to use.
	Image string `json:"image"`

	// Packages are the packages included in the image.
	Packages []string `json:"packages"`
}

// Options are the options for a build operation.
type Options struct {
	// Docker is the docker client to use.
	// Optional: if nil, attempts to create a docker client from the environment.
	Docker *client.Client

	// closeDocker is whether to close the docker client after use
	closeDocker bool

	// DockerImage is the docker image to use.
	DockerImage Image

	// Output is the OutputHandler to store the output to.
	Output OutputHandler

	// Packages is the PackageRetriever to use to fetch dependencies.
	Packages PackageRetriever

	// Dependencies is the DependencyFinder to use to search for dependencies.
	Dependencies DependencyFinder

	// Loader is a source loader for the build.
	// Must include the file:// loader for Build.
	// This loader should not have a file:// loader for BuildGraph.
	// This loader must implement buffering of variable-length data.
	Loader pkgen.Loader

	// Log is the log handler to use.
	// This field is ignored by BuildGraph.
	// If nil, defaults to buildlog.DefaultHandler.
	Log buildlog.Handler

	// Ctx is the context to use for the build.
	// If nil, defaults to context.Background()
	Ctx context.Context
}

func (o *Options) fix(pkg *pkgen.PackageGenerator) error {
	if o.Docker == nil {
		dcli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			return fmt.Errorf("failed to create docker client: %s", err.Error())
		}
		o.Docker = dcli
		o.closeDocker = true
	}
	if o.Log == nil {
		o.Log = buildlog.DefaultHandler
	}
	return nil
}

type dockerFileStream struct {
	tw *tar.Writer
}

// SendData copies the reader to docker with the given name.
// This must read exactly n bytes.
// If the reader is a *bytes.Buffer, n will be ignored.
func (dfs dockerFileStream) SendData(name string, r io.Reader, n int64) error {
	// check *bytes.Buffer length
	if buf, ok := r.(*bytes.Buffer); ok {
		n = int64(buf.Len())
	}

	// write tar header
	err := dfs.tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0644,
		Size: n,
	})
	if err != nil {
		return err
	}

	// write data to tar
	nw, err := io.Copy(dfs.tw, r)
	if err != nil {
		return err
	}

	// insufficient data
	if nw < n {
		return io.ErrUnexpectedEOF
	}

	return nil
}

// buildScript is a script to run in the container
var buildScript = []byte(`#!/bin/sh
set -e

# get into correct dir
cd $(dirname $0)

# install dependencies
if [ -e deps ]; then
	for i in $(cat deps/deps.list); do
		lpkg-inst $i
	end
fi

# run build
make -j8
`)

// Build builds a package.
func Build(pkg *pkgen.PackageGenerator, opts Options) (err error) {
	// prepare build configuration
	err = opts.fix(pkg)
	if err != nil {
		return err
	}
	if opts.closeDocker {
		defer func() {
			cerr := opts.Docker.Close()
			if cerr != nil && err == nil {
				err = cerr
			}
		}()
	}

	// add scope-cancelled context
	var cancel context.CancelFunc
	opts.Ctx, cancel = context.WithCancel(opts.Ctx)
	defer cancel()

	// create container
	err = opts.Log.Log(buildlog.Line{
		Stream: buildlog.StreamBuild,
		Text:   "Creating container. . .",
	})
	if err != nil {
		return err
	}
	containerCreate, err := opts.Docker.ContainerCreate(
		opts.Ctx,
		&container.Config{
			Image: opts.DockerImage.Image,
			Cmd:   []string{"/root/build/build.sh"},
		},
		nil, nil, "",
	)
	if err != nil {
		return err
	}
	defer func() {
		// remove container
		rctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cerr := opts.Docker.ContainerRemove(rctx, containerCreate.ID, types.ContainerRemoveOptions{
			Force: true,
		})
		if cerr != nil && err == nil {
			err = cerr
		}
	}()

	// prepare to create build inputs
	err = opts.Log.Log(buildlog.Line{
		Stream: buildlog.StreamBuild,
		Text:   "Preparing build payload. . .",
	})
	if err != nil {
		return err
	}

	// stream content to docker
	pr, pw := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()
	var dcerr error
	go func() {
		defer wg.Done()
		defer pr.Close()
		dcerr = opts.Docker.CopyToContainer(
			opts.Ctx,
			containerCreate.ID,
			"/root/build/",
			pr,
			types.CopyToContainerOptions{},
		)
	}()
	defer pw.Close()

	// generate tar data
	tw := tar.NewWriter(pw)

	// write source
	err = tw.WriteHeader(&tar.Header{
		Name:     "src",
		Mode:     0644 | int64(os.ModeDir),
		Typeflag: tar.TypeDir,
	})
	if err != nil {
		return err
	}
	err = pkg.WriteSourceTar(opts.Ctx, "src", tw, opts.Loader, 0)
	if err != nil {
		return err
	}

	// symlink Makefile into base dir
	err = tw.WriteHeader(&tar.Header{
		Name:     "Makefile",
		Mode:     0644 | int64(os.ModeSymlink),
		Typeflag: tar.TypeSymlink,
		Linkname: "src/Makefile",
	})
	if err != nil {
		return err
	}

	// send dependencies
	err = tw.WriteHeader(&tar.Header{
		Name:     "deps",
		Mode:     0644 | int64(os.ModeDir),
		Typeflag: tar.TypeDir,
	})
	if err != nil {
		return err
	}
	dlst := []string{}
	deps, err := opts.Dependencies.FindDependencies(pkg.BuildDependencies...)
	if err != nil {
		return err
	}
	for _, v := range deps {
		for _, p := range opts.DockerImage.Packages {
			if v == p {
				continue
			}
		}

		rc, l, err := opts.Packages.GetPkg(v, pkg.BuildArch)
		if err != nil {
			return err
		}

		name := filepath.Join("deps", v+".tar.gz")

		err = tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0644,
			Size: l,
		})
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(tw, rc)
		cerr := rc.Close()
		if err != nil {
			return err
		}
		if cerr != nil {
			return err
		}

		dlst = append(dlst, name)
	}
	dtxt := []byte(strings.Join(dlst, "\n"))
	err = tw.WriteHeader(&tar.Header{
		Name: "deps/deps.list",
		Mode: 0644,
		Size: int64(len(dtxt)),
	})
	if err != nil {
		return err
	}
	_, err = tw.Write(dtxt)
	if err != nil {
		return err
	}

	// inject build script
	err = tw.WriteHeader(&tar.Header{
		Name: "build.sh",
		Mode: 0744,
		Size: int64(len(buildScript)),
	})
	if err != nil {
		return err
	}
	_, err = tw.Write(buildScript)
	if err != nil {
		return err
	}

	// commit data to container filesystem
	err = tw.Close()
	if err != nil {
		return err
	}
	err = pw.Close()
	if err != nil {
		return err
	}
	wg.Wait()
	if dcerr != nil {
		return dcerr
	}

	// start build
	err = opts.Log.Log(buildlog.Line{
		Stream: buildlog.StreamBuild,
		Text:   "Starting build. . .",
	})
	if err != nil {
		return err
	}
	err = opts.Docker.ContainerStart(opts.Ctx, containerCreate.ID, types.ContainerStartOptions{})
	if err != nil {
		return err
	}

	// log build
	lr, err := opts.Docker.ContainerLogs(opts.Ctx, containerCreate.ID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	defer func() {
		cerr := lr.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	if err != nil {
		return err
	}
	opts.Log = buildlog.MutexedLogHandler(opts.Log)
	sow := buildlog.LogWriter(opts.Log, buildlog.StreamStdout)
	sew := buildlog.LogWriter(opts.Log, buildlog.StreamStderr)
	defer sow.Close()
	defer sew.Close()
	_, err = stdcopy.StdCopy(sow, sew, lr)
	if err != nil {
		return err
	}

	// check if completed ok
	info, err := opts.Docker.ContainerInspect(opts.Ctx, containerCreate.ID)
	if err != nil {
		return err
	}
	state := info.ContainerJSONBase.State
	if state.Running {
		return errors.New("container still running")
	}
	if state.ExitCode != 0 {
		return fmt.Errorf("build failed with exit code %d", state.ExitCode)
	}

	err = opts.Log.Log(buildlog.Line{
		Stream: buildlog.StreamBuild,
		Text:   "Transferring output. . .",
	})
	if err != nil {
		return err
	}

	// read build output
	drc, _, err := opts.Docker.CopyFromContainer(opts.Ctx, containerCreate.ID, "/root/build/pkgs.tar")
	if err != nil {
		return err
	}
	defer drc.Close()
	otr := tar.NewReader(drc)
	_, err = otr.Next()
	if err != nil {
		return err
	}
	tr := tar.NewReader(otr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				err = opts.Log.Log(buildlog.Line{
					Stream: buildlog.StreamBuild,
					Text:   "Build Complete!",
				})
				if err != nil {
					return err
				}
				return nil
			}
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		spl := strings.Split(filepath.Base(hdr.Name), ".")
		if len(spl) < 2 {
			return fmt.Errorf("found invalid output file %q", hdr.Name)
		}
		pkname := spl[0]
		err = opts.Output.Store(pkname, pkg.BuildArch, tr)
		if err != nil {
			return err
		}
	}
}
