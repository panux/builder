package buildmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/panux/builder/pkgen"
	"golang.org/x/tools/godoc/vfs"
)

// ErrPkgNotFound is an error type indicating that the specified package was not found.
type ErrPkgNotFound struct {
	PkgName string
}

func (err ErrPkgNotFound) Error() string {
	return fmt.Sprintf("package %q not found", err.PkgName)
}
func (err ErrPkgNotFound) String() string {
	return err.Error()
}

// RawPackageIndex is an in-memory index of packages.
type RawPackageIndex map[string]*RawPkent

// RawPkent is an entry in a RawPackageIndex.
type RawPkent struct {
	Path  string
	Pkgen *pkgen.RawPackageGenerator
}

// DepWalker returns a DepWalker function which resolves dependencies using the RawPackageIndex.
func (rpi RawPackageIndex) DepWalker() DepWalker {
	return func(p string) ([]string, error) {
		pe := rpi[p]
		if pe == nil {
			return nil, ErrPkgNotFound{p}
		}
		return pe.Pkgen.Packages[p].Dependencies, nil
	}
}

// List gets a list of packages.
func (rpi RawPackageIndex) List() []string {
	//get name list
	names := make([]string, len(rpi))
	i := 0
	for _, v := range rpi {
		names[i] = filepath.Base(filepath.Dir(v.Path))
		i++
	}

	//sort name list
	sort.Strings(names)

	//dedup name list
	for i = 1; i < len(names); {
		if names[i] == names[i-1] {
			names = names[:i+copy(names[i:], names[i+1:])]
		} else {
			i++
		}
	}

	return names
}

// indexEnts turns an array of RawPkent into a RawPackageIndex
func indexEnts(ents []*RawPkent) RawPackageIndex {
	rpi := make(RawPackageIndex)
	for _, ent := range ents {
		for p := range ent.Pkgen.Packages {
			rpi[p] = ent
		}
	}
	return rpi
}

// loadEnt loads a package entry.
func loadEnt(fs vfs.FileSystem, path string) (ent *RawPkent, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	rpg, err := pkgen.UnmarshalPkgen(f)
	if err != nil {
		return
	}
	return &RawPkent{
		Path:  path,
		Pkgen: rpg,
	}, nil
}

// FindPkgens finds all pkgens in a VFS.
func FindPkgens(dir vfs.FileSystem) ([]string, error) {
	return findPkgenV(dir, "/")
}

// findPkgenV is a recursive function to find pkgens in a VFS.
func findPkgenV(fs vfs.FileSystem, dir string) ([]string, error) {
	files, err := fs.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	pks := []string{}
	for _, f := range files {
		if f.IsDir() {
			subpks, err := findPkgenV(fs, filepath.Join(dir, f.Name()))
			if err != nil {
				return nil, err
			}
			pks = append(pks, subpks...)
		} else if filepath.Base(f.Name()) == "pkgen.yaml" {
			pks = append(pks, f.Name())
		}
	}
	return pks, nil
}

// IndexDir finds all pkgens in a dir and uses them to make a RawPackageIndex.
func IndexDir(dir vfs.FileSystem) (RawPackageIndex, error) {
	pkgens, err := FindPkgens(dir)
	if err != nil {
		return nil, err
	}
	ents := make([]*RawPkent, len(pkgens))
	for i, v := range pkgens {
		ent, err := loadEnt(dir, v)
		if err != nil {
			return nil, err
		}
		ents[i] = ent
	}
	return indexEnts(ents), nil
}
