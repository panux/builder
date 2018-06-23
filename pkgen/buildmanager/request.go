package buildmanager

import (
	"archive/tar"
	"context"
	"io"
	"strings"

	"gitlab.com/panux/builder/pkgen"
)

// BuildJobRequest is a BuildManager request container.
type BuildJobRequest struct {
	pk           *pkgen.PackageGenerator
	bdeps        []string
	pgetter      PackageRetriever
	loader       pkgen.Loader
	bootstrapped bool
	b            *Builder
}

// CreateBuildJobRequest creates a new BuildJobRequest.
func (b *Builder) CreateBuildJobRequest(pk *pkgen.PackageGenerator, bdeps []string, pget PackageRetriever, loader pkgen.Loader) (*BuildJobRequest, error) {
	return &BuildJobRequest{
		pk:      pk,
		bdeps:   bdeps,
		pgetter: pget,
		loader:  loader,
		b:       b,
	}, nil
}

// tar generates a tar of all of the necessary packages.
func (bjr *BuildJobRequest) tar(w io.Writer) (err error) {
	tw := tar.NewWriter(w)
	defer func() {
		cerr := tw.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	bdns := make([]string, len(bjr.bdeps))
	for i, d := range bjr.bdeps {
		var l uint32
		var r io.ReadCloser
		var ext string
		name, arch, bootstrap := parseJobName(d)
		bdns[i] = name
		l, r, ext, err = bjr.pgetter.GetPkg(name, arch, bootstrap)
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
			Name: "./" + name + ".tar." + ext,
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
	ilst := []byte(strings.Join(bdns, "\n"))
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

// writeSourceTar writes a tar of sources.
func (bjr *BuildJobRequest) writeSourceTar(ctx context.Context, w io.Writer) error {
	return bjr.pk.WriteSourceTar(ctx, w, bjr.loader, 100*1024*1024)
}
