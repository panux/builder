package bmapi

import (
	"io"
	"net/url"

	"github.com/panux/builder/pkgen"
	"golang.org/x/tools/godoc/vfs"
)

//FileLoadHandler is a tool to handle file load requests
type FileLoadHandler struct {
	w  *MsgStreamWriter
	fs vfs.FileSystem
}

//NewFileLoadHandler returns a new FileLoadHandler
func NewFileLoadHandler(w *MsgStreamWriter, fs vfs.FileSystem) *FileLoadHandler {
	return &FileLoadHandler{
		w:  w,
		fs: fs,
	}
}

//Handle proccesses a file request
func (flh *FileLoadHandler) Handle(m FileRequestMessage) (err error) {
	f, err := flh.fs.Open(m.Path)
	if err != nil {
		return
	}
	sw := flh.w.Stream(m.Stream)
	defer func() {
		cerr := sw.Close()
		if cerr != nil {
			if err == nil {
				err = cerr
			}
		}
	}()
	_, err = io.Copy(sw, f)
	return err
}

type fLoader struct {
	w *MsgStreamWriter
	r *MsgStreamReader
}

func (fl *fLoader) SupportedProtocols() ([]string, error) {
	return []string{"file"}, nil
}
func (fl *fLoader) Get(u *url.URL) (int64, io.ReadCloser, error) {
	if u.Scheme != "file" {
		return -1, nil, pkgen.ErrUnsupportedProtocol
	}
	strn, str := fl.r.Stream(2)
	err := fl.w.Send(FileRequestMessage{
		Path:   u.RawPath,
		Stream: strn,
	})
	if err != nil {
		str.Close()
		return -1, nil, err
	}
	return -1, str, nil
}

//NewFileRequestLoader returns a pkgen.Loader which sends FileRequestMessages to Get
func NewFileRequestLoader(w *MsgStreamWriter, r *MsgStreamReader) pkgen.Loader {
	return &fLoader{
		w: w,
		r: r,
	}
}
