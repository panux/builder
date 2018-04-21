package buildmanager

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/panux/builder/pkgen"
)

//ErrPkgNotFound is an error type indicating that the specified package was not found
type ErrPkgNotFound struct {
	PkgName string
}

func (err ErrPkgNotFound) Error() string {
	return fmt.Sprintf("package %q not found", err.PkgName)
}
func (err ErrPkgNotFound) String() string {
	return err.Error()
}

//RawPackageIndex is an in-memory index of packages
type RawPackageIndex map[string]*RawPkent

//RawPkent is an entry in a RawPackageIndex
type RawPkent struct {
	Path  string
	Pkgen *pkgen.RawPackageGenerator
}

//DepWalker returns a DepWalker function which resolves dependencies using the RawPackageIndex
func (rpi RawPackageIndex) DepWalker() DepWalker {
	return func(p string) ([]string, error) {
		pe := rpi[p]
		if pe == nil {
			return nil, ErrPkgNotFound{p}
		}
		return pe.Pkgen.Packages[p].Dependencies, nil
	}
}

//turn an array of RawPkent into a RawPackageIndex
func indexEnts(ents []*RawPkent) RawPackageIndex {
	rpi := make(RawPackageIndex)
	for _, ent := range ents {
		for p := range ent.Pkgen.Packages {
			rpi[p] = ent
		}
	}
	return rpi
}

func loadEnt(path string) (ent *RawPkent, err error) {
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

//FindPkgens finds all pkgens in a directory
func FindPkgens(dir string) ([]string, error) {
	pkgens := []string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".yaml" {
			pkgens = append(pkgens, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pkgens, nil
}

//IndexDir finds all pkgens in a dir and uses them to make a RawPackageIndex
func IndexDir(dir string) (RawPackageIndex, error) {
	pkgens, err := FindPkgens(dir)
	if err != nil {
		return nil, err
	}
	ents := make([]*RawPkent, len(pkgens))
	for i, v := range pkgens {
		ent, err := loadEnt(v)
		if err != nil {
			return nil, err
		}
		ents[i] = ent
	}
	return indexEnts(ents), nil
}
