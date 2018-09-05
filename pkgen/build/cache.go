package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/panux/builder/pkgen"
)

// HashCache is a cache of hashes
type HashCache struct {
	// m is a map of hashCacheKey to hashCacheEntries
	m map[hashCacheKey]*hashCacheEntry

	// pr is the PackageRetriever to hash packages from
	pr PackageRetriever

	// scan is the scan number of the cache
	scan uint64
}

// Clean prepares the HashCache for reuse.
// Any cache entries not used since the last call to Clean will be deleted.
// In addition, all entries will be timestamp-validated on their next use.
func (hc *HashCache) Clean() {
	// clean old entries
	for k, v := range hc.m {
		if v.scan != hc.scan {
			delete(hc.m, k)
		}
	}

	// move to new scan cycle
	hc.scan++
}

// hashCacheKey is a key type used for a HashCache
type hashCacheKey struct {
	// name is the name of the package
	name string

	// arch is the build architecture of the package
	arch pkgen.Arch
}

// hashCacheEntry is an entry in a HashCache
type hashCacheEntry struct {
	// hash is the generated hash
	hash [sha256.Size]byte

	// scan is the cache scan number
	// if scan == HashCache.scan, then the entry is automatically valid w/o timestamp checks
	scan uint64

	// timestamp is the last-modified time of the file on the last hash
	// if this matches the last-modified time of the file, the entry will be revalidated
	timestamp time.Time
}

// PackageHash hashes the corresponding package file.
// Cache entries will not be revalidated until the next call to Clean.
// After a call to Clean, a cache entry may be revalidated via timestamp.
// Timestamp caching is only available if the PackageRetriever returns a *os.File.
func (hc *HashCache) PackageHash(name string, arch pkgen.Arch) (hash [sha256.Size]byte, err error) {
	hck := hashCacheKey{
		name: name,
		arch: arch,
	}

	// lookup in cache
	hce := hc.m[hck]
	if hce != nil && hce.scan == hc.scan {
		return hce.hash, nil
	}
	if hce == nil {
		hce = new(hashCacheEntry)
		hc.m[hck] = hce
	}
	hce.scan = hc.scan

	defer func() {
		// flush cache entry on error
		if err != nil {
			delete(hc.m, hck)
		}
	}()

	// get package
	r, _, err := hc.pr.GetPkg(name, arch)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer func() {
		cerr := r.Close()
		if cerr != nil && err == nil {
			err = cerr
			hash = [sha256.Size]byte{}
		}
	}()

	// timestamp checking option
	if f, ok := r.(*os.File); ok {
		var inf os.FileInfo
		inf, err = f.Stat()
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		t := inf.ModTime()
		if t.Equal(hce.timestamp) {
			return hce.hash, nil
		}
		hce.timestamp = t
	}

	// hash package
	h := sha256.New()
	_, err = io.Copy(h, r)
	if err != nil {
		return [sha256.Size]byte{}, err
	}

	// store to cache
	h.Sum(hce.hash[:0])

	return hce.hash, nil
}

type hashRow struct {
	URL  string            `json:"url"`
	Hash [sha256.Size]byte `json:"hash"`
}

// hashSource hashes a source
func hashSource(ctx context.Context, url *url.URL, loader pkgen.Loader) (row hashRow, err error) {
	// create context with scoped cancellation
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// get source
	_, r, err := loader.Get(ctx, url)
	if err != nil {
		return hashRow{}, err
	}
	defer func() {
		cerr := r.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()

	// hash source
	h := sha256.New()
	_, err = io.Copy(h, r)
	if err != nil {
		return hashRow{}, err
	}

	// generate row
	row.URL = url.String()
	h.Sum(row.Hash[:0])

	return row, nil
}

// hashObjectJSON encodes the object to JSON and hashes the encoded data.
func hashObjectJSON(obj interface{}, dest *[sha256.Size]byte) error {
	h := sha256.New()
	err := json.NewEncoder(h).Encode(obj)
	if err != nil {
		return err
	}
	h.Sum(dest[:0])
	return nil
}

// HashPackage hashes the inputs of a package.
func HashPackage(ctx context.Context, pkg *pkgen.PackageGenerator, loader pkgen.Loader, hc *HashCache, docker Image, deps DependencyFinder) (hashhh [sha256.Size]byte, err error) {
	// hash pkgen
	tbl := []hashRow{
		hashRow{
			URL: "meta://pkgen.json",
		},
	}
	err = hashObjectJSON(pkg, &tbl[0].Hash)
	if err != nil {
		return [sha256.Size]byte{}, err
	}

	// add sources to table
	for _, s := range pkg.Sources {
		if s.Scheme == "file" {
			// hash files
			row, err := hashSource(ctx, s, loader)
			if err != nil {
				return [sha256.Size]byte{}, err
			}
			tbl = append(tbl, row)
		} else {
			// just put the URL for non-file sources
			tbl = append(tbl, hashRow{URL: s.String()})
		}
	}

	// find build dependencies
	dlst, err := deps.FindDependencies(pkg.BuildDependencies...)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
dloop:
	for _, d := range dlst {
		// skip packages in container
		for _, v := range docker.Packages {
			if v == d {
				continue dloop
			}
		}

		// get package hash
		hash, err := hc.PackageHash(d, pkg.BuildArch)
		if err != nil {
			return [sha256.Size]byte{}, err
		}

		// add package to table
		tbl = append(tbl, hashRow{
			URL:  "package://" + d + "/" + pkg.BuildArch.String(),
			Hash: hash,
		})
	}

	// add docker image to container
	if !strings.HasPrefix(docker.Image, "sha256:") {
		return [sha256.Size]byte{}, errors.New("docker image identifier is not in hash format")
	}
	dhash, err := hex.DecodeString(strings.TrimPrefix(docker.Image, "sha256:"))
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	if len(dhash) != sha256.Size {
		return [sha256.Size]byte{}, errors.New("malformed docker image identifier")
	}
	var drow hashRow
	copy(drow.Hash[:], dhash)
	drow.URL = "docker://" + strings.TrimPrefix(docker.Image, "sha256:")
	tbl = append(tbl, drow)

	// hash table
	var th [sha256.Size]byte
	err = hashObjectJSON(tbl, &th)
	if err != nil {
		return [sha256.Size]byte{}, err
	}

	return th, nil
}

// BuildCache is an interface used to cache builds.
type BuildCache interface {
	// Valid checks if the given build Info matches the cache entry.
	Valid(Info) (bool, error)

	// Update updates the cache with the given build Info.
	Update(Info) error
}

type dirJSONCache struct {
	dir string
}

func (jc dirJSONCache) Valid(info Info) (ok bool, err error) {
	f, err := os.Open(filepath.Join(jc.dir, info.PackageName+"-"+info.Arch.String()+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil && err == nil {
			err = cerr
			ok = false
		}
	}()

	var cinfo Info
	err = json.NewDecoder(f).Decode(&cinfo)
	if err != nil {
		return false, err
	}

	return info.Hash == cinfo.Hash, nil
}

func (jc dirJSONCache) Update(info Info) (err error) {
	f, err := os.OpenFile(filepath.Join(jc.dir, info.PackageName+"-"+info.Arch.String()+".json"), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()

	err = json.NewEncoder(f).Encode(info)
	if err != nil {
		return err
	}

	return nil
}

// DirJSONCache creates a BuildCache storing JSON blobs in the dir.
func DirJSONCache(dir string) BuildCache {
	return dirJSONCache{dir: dir}
}
