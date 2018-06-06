package pkgen

import (
	"fmt"
	"io"
	"strings"
)

// PkgInfo is a container with the data in a .pkginfo.
type PkgInfo struct {
	Name         string
	Version      string
	Dependencies []string
}

// WriteTo writes a PkgInfo (implements io.WriterTo).
func (pki PkgInfo) WriteTo(w io.Writer) (int64, error) {
	n1, err := fmt.Fprintf(w, "NAME=%q\nVERSION=%q\n", pki.Name, pki.Version)
	if err != nil {
		return int64(n1), err
	}
	var n2 int
	if len(pki.Dependencies) > 0 {
		n2, err = fmt.Fprintf(w, "DEPENDENCIES=%q\n", strings.Join(pki.Dependencies, " "))
		if err != nil {
			return int64(n1 + n2), err
		}
	}
	return int64(n1 + n2), nil
}

// PackageInfos returns a set of PkgInfo for the PackageGenerator.
func (pg *PackageGenerator) PackageInfos() []PkgInfo {
	infos := make([]PkgInfo, len(pg.Packages))
	for i, v := range pg.ListPackages() {
		infos[i] = PkgInfo{
			Name:         v,
			Version:      pg.Version,
			Dependencies: pg.Packages[v].Dependencies,
		}
	}
	return infos
}
