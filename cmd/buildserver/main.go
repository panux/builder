package main

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/panux/builder/pkgen"
)

func main() {
}

//get a hash of the pkgen and all related files
func resHash(pk *pkgen.PackageGenerator, ldr pkgen.Loader) ([]byte, error) {
	//hash sources
	fls := make(map[string][]byte)
	for _, v := range pk.Sources {
		if v.Scheme == "file" {
			_, r, err := ldr.Get(v)
			if err != nil {
				return nil, err
			}
			h := sha256.New()
			_, err = io.Copy(h, r)
			if err != nil {
				return nil, err
			}
			fls[v.String()] = h.Sum(nil)
		}
	}
	//hash pkgen
	h := sha256.New()
	err := json.NewEncoder(h).Encode(pk)
	if err != nil {
		return nil, err
	}
	fls["file://./pkgen.yaml"] = h.Sum(nil)
	//convert to a serializable format
	us := make([]string, len(fls))
	i := 0
	for s := range fls {
		us[i] = s
	}
	sort.Strings(us)
	pairs := make([]fhpair, len(fls))
	for i, u := range us {
		pairs[i] = fhpair{
			URL:  u,
			Hash: fls[u],
		}
	}
	//hash JSON of pairs
	h.Reset()
	err = json.NewEncoder(h).Encode(pairs)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

type fhpair struct {
	URL  string `json:"url"`
	Hash []byte `json:"hash"`
}

//find all pkgens in a dir
func findPkgens(dir string) ([]string, error) {
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

//dir containing build info files
var binfodir string

//buildInfo is a JSON format used for storing build info in files
type buildInfo struct {
	PackageName string                 `json:"packageName"`
	InputHash   []byte                 `json:"sha256"`
	Pkgen       pkgen.PackageGenerator `json:"pkgen"`
	Log         []logEntry             `json:"log,omitempty"`
}

//logEntry is the JSON representation of a line of log
type logEntry struct {
	Text   string `json:"text"`
	Stream int    `json:"stream"`
}
