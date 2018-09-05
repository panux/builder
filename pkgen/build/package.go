package build

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"

	"gitlab.com/panux/builder/pkgen"
)

// OutputHandler is an interface to handle the output of builds.
type OutputHandler interface {
	// Store stores the output of a build to an external location.
	// The body is in tar format, containing the packages with path in relative format (./example.tar.gz).
	Store(name string, arch pkgen.Arch, body io.Reader) error
}

// PackageRetriever is an interface to load packages.
type PackageRetriever interface {
	// GetPkg gets a package with the given name and arch in tar format.
	// Also must return length of stream.
	GetPkg(name string, arch pkgen.Arch) (io.ReadCloser, int64, error)
}

// PackageStore is an interface for storing and retrieving packages.
type PackageStore interface {
	OutputHandler
	PackageRetriever
}

type dirStore struct {
	dir string
}

func (ds dirStore) writeFile(name string, src io.Reader) (err error) {
	// generate path
	path := filepath.Join(ds.dir, name)

	// open file
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()

	// copy to file
	_, err = io.Copy(f, src)
	if err != nil {
		return err
	}

	return nil
}

func (ds dirStore) Store(name string, arch pkgen.Arch, body io.Reader) (err error) {
	f, err := os.OpenFile(filepath.Join(ds.dir, name+"-"+arch.String()+".tar.gz"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
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
	if err != nil {
		return err
	}

	return nil
}

func (ds dirStore) GetPkg(name string, arch pkgen.Arch) (io.ReadCloser, int64, error) {
	// check arch validity
	if !arch.Supported() {
		return nil, -1, pkgen.ErrUnsupportedArch
	}

	// generate path
	path := filepath.Join(ds.dir, name+"-"+arch.String()+".tar.gz")

	// get file
	f, err := os.Open(path)
	if err != nil {
		return nil, -1, err
	}

	// get file size
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, -1, err
	}

	return f, info.Size(), nil
}

// DirStore creates a PackageStore which stores packages in a directory.
func DirStore(dir string) PackageStore {
	return &dirStore{dir: dir}
}

// Info is a struct containing identifying information for the build.
type Info struct {
	// PackageName is the name of the package being built.
	PackageName string `json:"name"`

	// Arch is the arch for which the build is being run.
	Arch pkgen.Arch `json:"arch"`

	// Hash is the SHA256 hash of the build inputs.
	Hash [sha256.Size]byte `json:"hash"`
}

// BuildDepsDocker finds build deps not provided by docker.
func BuildDepsDocker(pkg *pkgen.PackageGenerator, deps DependencyFinder, img Image) ([]string, error) {
	d, err := deps.FindDependencies(pkg.BuildDependencies...)
	if err != nil {
		return nil, err
	}

	d2 := []string{}
	for _, v := range d {
		for _, p := range img.Packages {
			if v == p {
				continue
			}

			d2 = append(d, v)
		}
	}

	return d2, nil
}

func mapRuleDeps(rpi RawPackageIndex, arch pkgen.Arch, deps ...string) []string {
	rdeps := map[string]struct{}{}
	for _, d := range deps {
		rdeps[filepath.Base(filepath.Dir(rpi[d].Path))] = struct{}{}
	}

	res := make([]string, len(rdeps))
	i := 0
	for d := range rdeps {
		res[i] = d + "-" + arch.String()
	}

	return res
}
