package pkgen

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"path/filepath"
)

//WriteSourceTar creates a tar file containing all of the source files necessary for building a package
//Also includes the Makefile
//May buffer files of unknown size up to maxbuf bytes in memory
func (pg *PackageGenerator) WriteSourceTar(w io.Writer, loader Loader, maxbuf uint) (err error) {
	tw := tar.NewWriter(w)
	defer func() {
		cerr := tw.Close()
		if err == nil {
			err = cerr
		}
	}()
	{
		buf := bytes.NewBuffer(nil)
		_, err = pg.GenFullMakefile(DefaultVars).WriteTo(buf)
		if err != nil {
			return
		}
		err = tw.WriteHeader(&tar.Header{
			Name: "Makefile",
			Mode: 0600,
			Size: int64(buf.Len()),
		})
		if err != nil {
			return
		}
		_, err = buf.WriteTo(tw)
		if err != nil {
			return
		}
	}
	for _, inf := range pg.PackageInfos() {
		var buf bytes.Buffer
		_, err = inf.WriteTo(&buf)
		if err != nil {
			return
		}
		err = tw.WriteHeader(&tar.Header{
			Name: inf.Name + ".pkginfo",
			Mode: 0600,
			Size: int64(buf.Len()),
		})
		if err != nil {
			return
		}
		_, err = buf.WriteTo(tw)
		if err != nil {
			return
		}
	}
	for _, s := range pg.Sources {
		var l int64
		var r io.ReadCloser
		l, r, err = loader.Get(s)
		if err != nil {
			return
		}
		defer func() {
			cerr := r.Close()
			if err == nil {
				err = cerr
			}
		}()
		if l < 1 { //indefinite size - buffer in memory to get size
			b := bytes.NewBuffer(nil)
			mr := maxReader{
				r: r,
				n: maxbuf,
			}
			_, err = io.Copy(b, &mr)
			if err != nil {
				return
			}
			l = int64(b.Len())
			r = ioutil.NopCloser(b)
		}
		err = tw.WriteHeader(&tar.Header{
			Name: filepath.Base(s.Path),
			Mode: 0600,
			Size: l,
		})
		if err != nil {
			return
		}
		_, err = io.Copy(tw, r)
		if err != nil {
			return
		}
	}
	return
}
