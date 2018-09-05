package build

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gitlab.com/panux/builder/pkgen"
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
func (rpi RawPackageIndex) DepWalker(pkg string) ([]string, error) {
	// lookup package in index
	ent, ok := rpi[pkg]
	if !ok {
		return nil, ErrPkgNotFound{pkg}
	}

	// get deps
	return ent.Pkgen.Packages[pkg].Dependencies, nil
}

// FindDependencies finds the dependencies of the given packages recursively
func (rpi RawPackageIndex) FindDependencies(pkgs ...string) ([]string, error) {
	return DepWalker(rpi.DepWalker).Walk(pkgs...)
}

// List gets a list of packages.
func (rpi RawPackageIndex) List() []string {
	// get name list
	names := make([]string, len(rpi))
	i := 0
	for _, v := range rpi {
		names[i] = filepath.Base(filepath.Dir(v.Path))
		i++
	}

	// sort name list
	sort.Strings(names)

	// dedup name list
	for i = 1; i < len(names); {
		if names[i] == names[i-1] {
			names = names[:i+copy(names[i:], names[i+1:])]
		} else {
			i++
		}
	}

	return names
}

func (rpi RawPackageIndex) addPkent(ent *RawPkent) {
	for p := range ent.Pkgen.Packages {
		rpi[p] = ent
	}
	rpi[filepath.Base(filepath.Dir(ent.Path))] = ent
}

// loadEnt loads a package entry.
func loadEnt(fs vfs.FileSystem, path string) (ent *RawPkent, err error) {
	f, err := fs.Open(path)
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

// indexVFS recurses through a VFS and adds all pkgen.yaml files to the RawPackageIndex.
// The top level call should pass in nil for info.
func indexVFS(fs vfs.FileSystem, path string, info os.FileInfo, rpi RawPackageIndex) error {
	switch {
	case info == nil:
		// top level
		fallthrough
	case info.IsDir():
		// recurse into directory
		files, err := fs.ReadDir(path)
		if err != nil {
			return err
		}
		for _, f := range files {
			err = indexVFS(fs, filepath.Join(path, f.Name()), f, rpi)
			if err != nil {
				return err
			}
		}
	case info.Name() == "pkgen.yaml":
		// load pkgen
		ent, err := loadEnt(fs, path)
		if err != nil {
			return err
		}

		// add entry to index
		rpi.addPkent(ent)
	}

	return nil
}

// IndexDir finds all pkgens in a dir and uses them to make a RawPackageIndex.
func IndexDir(dir vfs.FileSystem) (RawPackageIndex, error) {
	// create index
	rpi := make(RawPackageIndex)

	// recurse through dir
	err := indexVFS(dir, ".", nil, rpi)
	if err != nil {
		return nil, err
	}

	return rpi, nil
}
