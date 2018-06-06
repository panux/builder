package buildmanager

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/panux/builder/pkgen"
)

// BuildInfo is a struct containing identifying information for the build.
type BuildInfo struct {
	// PackageName is the name of the package being built.
	PackageName string `json:"name"`

	// Arch is the arch for which the build is being run.
	Arch pkgen.Arch `json:"arch"`

	// Bootstrap indicates whether or not this is a bootstrap build.
	Bootstrap bool `json:"bootstrap"`

	// Hash is the SHA256 hash of the build inputs.
	Hash [sha256.Size]byte `json:"hash"`
}

// BuildCache is an interface to check whether builds are up to date.
// Must be concurrency-safe.
type BuildCache interface {
	// CheckLatest checks if the BuildInfo matches the current version.
	CheckLatest(BuildInfo) (bool, error)

	// UpdateCache updates a cache entry for a BuildInfo.
	UpdateCache(BuildInfo) error
}

// jsonDirCache is a BuildCache which uses a dir of JSON blobs.
type jsonDirCache struct {
	lck sync.Mutex
	dir string
}

func (jdc *jsonDirCache) CheckLatest(b BuildInfo) (bool, error) {
	//lock to avoid unsafe access
	jdc.lck.Lock()
	defer jdc.lck.Unlock()

	//open file
	suf := ""
	if b.Bootstrap {
		suf = "-bootstrap"
	}
	f, err := os.Open(filepath.Join(jdc.dir, filepath.Clean(fmt.Sprintf("%s-%s%s.json", b.PackageName, b.Arch.String(), suf))))
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()

	//decode JSON
	var obi BuildInfo
	err = json.NewDecoder(f).Decode(&obi)
	if err != nil {
		return false, err
	}

	//compare
	return b == obi, nil
}

func (jdc *jsonDirCache) UpdateCache(b BuildInfo) (err error) {
	//lock to avoid unsafe access
	jdc.lck.Lock()
	defer jdc.lck.Unlock()

	//open file
	suf := ""
	if b.Bootstrap {
		suf = "-bootstrap"
	}
	f, err := os.OpenFile(filepath.Join(jdc.dir, filepath.Clean(fmt.Sprintf("%s-%s%s.json", b.PackageName, b.Arch.String(), suf))), os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()

	//store JSON
	return json.NewEncoder(f).Encode(b)
}

// NewJSONDirCache creates a BuildCache which uses a vfs.
func NewJSONDirCache(dir string) BuildCache {
	return &jsonDirCache{dir: dir}
}