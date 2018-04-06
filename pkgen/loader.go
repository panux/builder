package pkgen

import (
	"errors"
	"io"
	"net/url"
	"sort"
)

//Loader is an interface for source loaders
type Loader interface {
	SupportedProtocols() []string
	Get(*url.URL) (io.ReadCloser, error)
}

type multiLoader struct {
	loaders map[string]Loader
	protos  []string
}

//ErrUnsupportedProtocol is returned by Loader.Get if the protocol of the URL is unsupported
var ErrUnsupportedProtocol = errors.New("unsupported protocol")

//ErrExceedsMaxBuffer is an error returned by Loader.Get if the resource is too big to be buffered
var ErrExceedsMaxBuffer = errors.New("resource exceeds maximum buffer size")

//ErrMissingHash is an error returned by Loader.Get if the resource is being loaded over an insecure protocol and does not have a hash
var ErrMissingHash = errors.New("insecure resource does not have hash")

func (ml *multiLoader) SupportedProtocols() []string {
	return ml.protos
}
func (ml *multiLoader) Get(u *url.URL) (io.ReadCloser, error) {
	nl := ml.loaders[u.Scheme]
	if nl == nil {
		return nil, ErrUnsupportedProtocol
	}
	return nl.Get(u)
}

//NewMultiLoader returns a Loader which uses the input loaders
//SupportedProtocols is the union of the SupportedProtocols sets from loaders
//If multiple loaders support the same protocol, the last one will be used
//If no loaders are input, NewMultiLoader will return nil
func NewMultiLoader(loaders ...Loader) Loader {
	if len(loaders) == 0 {
		return nil
	}
	ldm := make(map[string]Loader) //generate map of scheme to NetLoader
	for _, l := range loaders {
		protos := l.SupportedProtocols()
		for _, p := range protos {
			ldm[p] = l
		}
	}
	pr := make([]string, len(ldm)) //create a list of supported schemes
	i := 0
	for p := range ldm {
		pr[i] = p
		i++
	}
	sort.Strings(pr) //sort the scheme list just for consistency
	ml := new(multiLoader)
	ml.loaders = ldm
	ml.protos = pr
	return ml
}
