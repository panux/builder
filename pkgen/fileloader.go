package pkgen

import (
	"context"
	"io"
	"net/url"

	"golang.org/x/tools/godoc/vfs"
)

// fileLoader is a Loader which loads files from a VFS.
type fileLoader struct {
	fs vfs.FileSystem
}

var fprotos = []string{"file"}

func (fl fileLoader) SupportedProtocols() ([]string, error) {
	return fprotos, nil
}

func (fl fileLoader) Get(ctx context.Context, u *url.URL) (int64, io.ReadCloser, error) {
	var l int64 = -1
	info, err := fl.fs.Stat(u.Path)
	if err == nil {
		l = info.Size()
	}
	f, err := fl.fs.Open(u.Path)
	if err != nil {
		return -1, nil, err
	}
	return l, f, nil
}

// FileLoader returns a Loader which loads files from the given VFS.
func FileLoader(fs vfs.FileSystem) Loader {
	return fileLoader{fs: fs}
}
