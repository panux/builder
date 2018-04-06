package pkgen

import (
	"io"
	"net/url"

	"golang.org/x/tools/godoc/vfs"
)

type fileLoader struct {
	fs vfs.FileSystem
}

var fprotos = []string{"file"}

func (fl fileLoader) SupportedProtocols() []string {
	return fprotos
}

func (fl fileLoader) Get(u *url.URL) (io.ReadCloser, error) {
	return fl.fs.Open(u.Path)
}

//NewFileLoader returns a new Loader which loads files from the given VFS
func NewFileLoader(fs vfs.FileSystem) Loader {
	return fileLoader{fs: fs}
}
