package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/panux/builder/pkgen"
	"gitlab.com/panux/builder/pkgen/buildmanager"
)

// PackageStore is a package storage/retrieval system.
type PackageStore struct {
	dir string
}

// genDir creates a directory if it does not already exist.
func genDir(dir string) error {
	err := os.Mkdir(dir, 0755)
	if os.IsExist(err) {
		err = nil
	}
	return err
}

// filePath determines the path associated with the tar with the given build information.
func (ps *PackageStore) filePath(filename string, bi buildmanager.BuildInfo) string {
	suf := ""
	if bi.Bootstrap {
		suf = "-bootstrap"
	}
	return filepath.Join(
		ps.dir,
		fmt.Sprintf("%s-%s%s.tar.gz",
			strings.TrimSuffix(filename, ".tar.gz"),
			bi.Arch,
			suf,
		),
	)
}

// Store attempts to store a package file (implements buildmanager.OutputHandler).
func (ps *PackageStore) Store(build buildmanager.BuildInfo, filename string, body io.ReadCloser) (err error) {
	f, err := os.OpenFile(ps.filePath(filename, build), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(f, body)
	return err
}

// GetPkg attempts to retrieve a package with the given name and arch.
func (ps *PackageStore) GetPkg(name string, arch pkgen.Arch, bootstrap bool) (len uint32, r io.ReadCloser, ext string, err error) {
	f, err := os.Open(ps.filePath(name, buildmanager.BuildInfo{
		Arch:      arch,
		Bootstrap: bootstrap,
	}))
	if err != nil {
		return 0, nil, "", err
	}
	inf, err := f.Stat()
	if err != nil {
		return 0, nil, "", err
	}
	return uint32(inf.Size()), f, "gz", nil
}
