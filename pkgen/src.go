package pkgen

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"path/filepath"
)

// WriteSourceTar creates a tar file containing all of the source files necessary for building a package.
// Also includes the Makefile in the tar.
// May buffer files of unknown size up to maxbuf bytes in memory.
// Context may be used for cancellation of internal steps.
// Closing of the underlying io.Writer is necessary to garuntee cancellation.
func (pg *PackageGenerator) WriteSourceTar(ctx context.Context, path string, tw *tar.Writer, loader Loader, maxbuf int64) (err error) {
	// handle cancellation errors
	defer func() {
		if ctxerr := ctx.Err(); ctxerr != nil {
			err = ctxerr
		}
	}()

	// wrap loader for in-memory buffering
	loader = BufferLoader(loader, maxbuf)

	// generate Makefile
	buf := bytes.NewBuffer(nil)
	_, err = pg.GenFullMakefile(DefaultVars).WriteTo(buf)
	if err != nil {
		return err
	}
	err = tw.WriteHeader(&tar.Header{
		Name: filepath.Join(path, "Makefile"),
		Mode: 0600,
		Size: int64(buf.Len()),
	})
	if err != nil {
		return err
	}
	_, err = buf.WriteTo(tw)
	if err != nil {
		return err
	}

	// generate package info files
	for _, inf := range pg.PackageInfos() {
		var buf bytes.Buffer
		_, err = inf.WriteTo(&buf)
		if err != nil {
			return err
		}
		err = tw.WriteHeader(&tar.Header{
			Name: filepath.Join(path, inf.Name+".pkginfo"),
			Mode: 0600,
			Size: int64(buf.Len()),
		})
		if err != nil {
			return err
		}
		_, err = buf.WriteTo(tw)
		if err != nil {
			return err
		}
	}

	// get and tar sources
	for _, s := range pg.Sources {
		// run Get
		var l int64
		var r io.ReadCloser
		l, r, err = loader.Get(ctx, s)
		if err != nil {
			return err
		}
		defer func() {
			cerr := r.Close()
			if err == nil {
				err = cerr
			}
		}()

		// store source into tar
		err = tw.WriteHeader(&tar.Header{
			Name: filepath.Join(path, filepath.Base(s.Path)),
			Mode: 0600,
			Size: l,
		})
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, r)
		if err != nil {
			return err
		}
	}

	return nil
}
