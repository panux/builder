package buildmanager

import (
	"archive/tar"
	"io"
	"strings"

	"github.com/panux/builder/pkgen"
)

//PackageGetter is a function type called to get a package
type PackageGetter func(path string) (len uint32, r io.ReadCloser, ext string, err error)

//BuildJobRequest is a thing
type BuildJobRequest struct {
	pk      *pkgen.PackageGenerator
	bdeps   []string
	pgetter PackageGetter
}

//CreateBuildJobRequest creates a new BuildJobRequest
func CreateBuildJobRequest(pk *pkgen.PackageGenerator, dw DepWalker, pget PackageGetter) (*BuildJobRequest, error) {
	var bdeps []string
	var err error
	if pk.Builder != "bootstrap" {
		bdeps, err = dw.Walk(append(pk.BuildDependencies, "base-build")...)
		if err != nil {
			return nil, err
		}
	}
	return &BuildJobRequest{
		pk:      pk,
		bdeps:   bdeps,
		pgetter: pget,
	}, nil
}

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
		l, r, ext, err = bjr.pgetter(d)
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
