package buildmanager

import (
	"archive/tar"
	"context"
	"io"
	"strings"

	"github.com/panux/builder/pkgen"
)

// BuildJobRequest is a BuildManager request container
type BuildJobRequest struct {
	pk           *pkgen.PackageGenerator
	bdeps        []string
	pgetter      PackageRetriever
	loader       pkgen.Loader
	bootstrapped bool
}

//CreateBuildJobRequest creates a new BuildJobRequest
func CreateBuildJobRequest(pk *pkgen.PackageGenerator, dw DepWalker, pget PackageRetriever, loader pkgen.Loader) (*BuildJobRequest, error) {
	var bdeps []string
	var err error
	if pk.Builder != "bootstrap" {
		bdeps, err = dw.Walk(append(pk.BuildDependencies, "base-build")...)
		if err != nil {
			return nil, err
		}
	} else {
		bdeps = []string{}
	}
	return &BuildJobRequest{
		pk:      pk,
		bdeps:   bdeps,
		pgetter: pget,
		loader:  loader,
	}, nil
}

// tar generates a tar of all of the necessary packages
func (bjr *BuildJobRequest) tar(w io.Writer) (err error) {
	tw := tar.NewWriter(w)
	defer func() {
		cerr := tw.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	for _, d := range bjr.bdeps {
		var l uint32
		var r io.ReadCloser
		var ext string
		l, r, ext, err = bjr.pgetter.GetPkg(d, bjr.pk.HostArch, bjr.bootstrapped)
		if err != nil {
			return
		}
		defer func() {
			if r != nil {
				cerr := r.Close()
				if cerr != nil && err == nil {
					err = cerr
				}
			}
		}()
		err = tw.WriteHeader(&tar.Header{
			Name: "./" + d + ".tar" + ext,
			Mode: 0644,
			Size: int64(l),
		})
		if err != nil {
			return
		}
		_, err = io.Copy(tw, io.LimitReader(r, int64(l)))
		if err != nil {
			return
		}
		err = r.Close()
		r = nil
		if err != nil {
			return
		}
	}
	ilst := []byte(strings.Join(bjr.bdeps, "\n"))
	err = tw.WriteHeader(&tar.Header{
		Name: "./inst.list",
		Mode: 0644,
		Size: int64(len(ilst)),
	})
	if err != nil {
		return
	}
	_, err = tw.Write(ilst)
	if err != nil {
		return
	}
	return
}

// writeSourceTar writes a tar of sources
func (bjr *BuildJobRequest) writeSourceTar(ctx context.Context, w io.Writer) error {
	return bjr.pk.WriteSourceTar(ctx, w, bjr.loader, 100*1024*1024)
}
