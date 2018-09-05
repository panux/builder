package pkgen

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
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
	// The int64 is the length of the source. Lengths less than 0 indicate that the length is unknown.
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

// MultiLoader returns a Loader which uses the input loaders.
// SupportedProtocols is the union of the SupportedProtocols sets from loaders.
// If multiple loaders support the same protocol, the last one will be used.
// If no loaders are input, MultiLoader will return nil.
func MultiLoader(loaders ...Loader) (Loader, error) {
	// generate map of scheme to Loader
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

	// create a list of supported schemes
	pr := make([]string, len(ldm))
	i := 0
	for p := range ldm {
		pr[i] = p
		i++
	}

	// sort the scheme list for consistency
	sort.Strings(pr)
	ml := new(multiLoader)
	ml.loaders = ldm
	ml.protos = pr

	return ml, nil
}

// ErrExceedsMaxBuffer is an error returned by Loader.Get if the resource is too big to be buffered.
var ErrExceedsMaxBuffer = errors.New("resource exceeds maximum buffer size")

type lenBufferLoader struct {
	Loader
	maxBuffer int64
}

func (lbl *lenBufferLoader) Get(ctx context.Context, u *url.URL) (n int64, rc io.ReadCloser, err error) {
	// get from underlying loader
	n, r, err := lbl.Loader.Get(ctx, u)
	if err != nil || n >= 0 {
		return n, r, err
	}

	// close reader when done
	defer func() {
		cerr := r.Close()
		if cerr != nil && err == nil {
			err = cerr
			n = 0
			rc = nil
		}
	}()

	// copy to buffer
	var buf bytes.Buffer
	lr := io.LimitedReader{
		R: r,
		N: lbl.maxBuffer,
	}
	_, err = io.Copy(&buf, &lr)
	if err != nil {
		return 0, nil, err
	}
	if lr.N <= 0 {
		return 0, nil, ErrExceedsMaxBuffer
	}

	// return buffer
	return int64(buf.Len()), ioutil.NopCloser(&buf), nil
}

// BufferLoader returns a Loader that will always provide a length.
// If no length is provided by the underlying Loader, it will buffer the data in memory.
// If the data is buffered and size exceeds maxBuffer, it will return ErrExceedsMaxBuffer.
// If loader is already a BufferLoader, loader will be returned.
func BufferLoader(loader Loader, maxBuffer int64) Loader {
	if _, ok := loader.(*lenBufferLoader); ok {
		return loader
	}
	return &lenBufferLoader{
		Loader:    loader,
		maxBuffer: maxBuffer,
	}
}
