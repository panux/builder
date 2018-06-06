package pkgen

import (
	"context"
	"errors"
	"io"
	"net/url"
	"sort"
)

// Loader is an interface for source loaders.
type Loader interface {
	// SupportedProtocols gets a list of protocols supported by the Loader.
	// These protocols are used as URL schemes.
	SupportedProtocols() ([]string, error)

	// Get retrieves a source with the given URL.
	// Implementations may optionally use the context with cancellation.
	// The caller must cancel the context at some point in time.
	// The returned io.ReadCloser contains the content, and the caller must close this.
	// The int64 is the length of the source. Lengths less than 1 indicate that the length is unknown.
	// If the protocol is unsupported,  Get should return ErrUnsupportedProtocol.
	Get(context.Context, *url.URL) (int64, io.ReadCloser, error)
}

// multiLoader is a loader that uses a group of other loaders to load sources.
type multiLoader struct {
	loaders map[string]Loader
	protos  []string
}

// ErrUnsupportedProtocol is returned by Loader.Get if the protocol of the URL is unsupported.
var ErrUnsupportedProtocol = errors.New("unsupported protocol")

// ErrExceedsMaxBuffer is an error returned by Loader.Get if the resource is too big to be buffered.
var ErrExceedsMaxBuffer = errors.New("resource exceeds maximum buffer size")

// ErrMissingHash is an error returned by Loader.Get if the resource is being loaded over an insecure protocol and does not have a hash.
var ErrMissingHash = errors.New("insecure resource does not have hash")

func (ml *multiLoader) SupportedProtocols() ([]string, error) {
	return ml.protos, nil
}
func (ml *multiLoader) Get(ctx context.Context, u *url.URL) (int64, io.ReadCloser, error) {
	nl := ml.loaders[u.Scheme]
	if nl == nil {
		return -1, nil, ErrUnsupportedProtocol
	}
	return nl.Get(ctx, u)
}

// NewMultiLoader returns a Loader which uses the input loaders.
// SupportedProtocols is the union of the SupportedProtocols sets from loaders.
// If multiple loaders support the same protocol, the last one will be used.
// If no loaders are input, NewMultiLoader will return nil.
func NewMultiLoader(loaders ...Loader) (Loader, error) {
	//generate map of scheme to Loader
	ldm := make(map[string]Loader)
	for _, l := range loaders {
		protos, err := l.SupportedProtocols()
		if err != nil {
			return nil, err
		}
		for _, p := range protos {
			ldm[p] = l
		}
	}

	//create a list of supported schemes
	pr := make([]string, len(ldm))
	i := 0
	for p := range ldm {
		pr[i] = p
		i++
	}

	//sort the scheme list for consistency
	sort.Strings(pr)
	ml := new(multiLoader)
	ml.loaders = ldm
	ml.protos = pr

	return ml, nil
}
