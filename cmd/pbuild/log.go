package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"gitlab.com/jadr2ddude/sse"
	"gitlab.com/panux/builder/pkgen/buildlog"
	"gitlab.com/panux/builder/pkgen/buildmanager"
)

// LogStream is an interface for a log reading mechanism.
type LogStream interface {
	// NextLine gets the next line in the log.
	// If this is the end of the log, io.EOF will be returned.
	NextLine() (buildlog.Line, error)

	// Close closes the LogStream.
	Close()
}

// LogStore is a storage directory of logs.
type LogStore struct {
	path string
}

// Save saves a log to a file in JSON.
func (ls *LogStore) Save(bi buildmanager.BuildInfo, ll []buildlog.Line) (err error) {
	fname := fmt.Sprintf("%x.json", bi.Hash)
	path := filepath.Join(ls.path, fname)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	err = json.NewEncoder(f).Encode(ll)
	if err != nil {
		return err
	}
	return nil
}

// GetLog gets an io.ReadCloser which reads the log as a JSON []buildlog.Line.
func (ls *LogStore) GetLog(bi buildmanager.BuildInfo) (log []buildlog.Line, err error) {
	f, err := os.Open(filepath.Join(ls.path, fmt.Sprintf("%x.json", bi.Hash)))
	if err != nil {
		return nil, err
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	err = json.NewDecoder(f).Decode(&log)
	if err != nil {
		return nil, err
	}
	return log, nil
}

// arrayLogStream is a LogStream that reads from a pre-generated log in an array.
type arrayLogStream struct {
	log []buildlog.Line
}

func (als *arrayLogStream) NextLine() (buildlog.Line, error) {
	if len(als.log) == 0 {
		return buildlog.Line{}, io.EOF
	}
	line := als.log[0]
	als.log = als.log[1:]
	return line, nil
}

func (als *arrayLogStream) Close() {
	als.log = nil
}

// Stream returns a LogStream that reads from a file.
func (ls *LogStore) Stream(bi buildmanager.BuildInfo) (LogStream, error) {
	log, err := ls.GetLog(bi)
	if err != nil {
		return nil, err
	}
	return &arrayLogStream{log}, nil
}

// LogSession is a logging system for one build.
type LogSession struct {
	wg        sync.WaitGroup
	in        <-chan buildlog.Line
	subscribe chan chan<- buildlog.Line
	store     *LogStore
	bi        buildmanager.BuildInfo
	err       error
}

// chanLogStream is a LogStream implementation using a channel to read log lines.
type chanLogStream chan buildlog.Line

func (ch chanLogStream) NextLine() (buildlog.Line, error) {
	line, ok := <-ch
	if !ok {
		return buildlog.Line{}, io.EOF
	}
	return line, nil
}

func (ch chanLogStream) Close() {
	defer func() { recover() }()
	close(ch)
}

// trySubscribe attempts to subscribe to the *LogSession and returns whether it was successful.
func (ls *LogSession) trySubscribe(lch chan<- buildlog.Line) bool {
	defer func() { recover() }()
	ls.subscribe <- lch
	return true
}

// ErrSessionClosed indicates that the session has been closed.
var ErrSessionClosed = errors.New("session closed")

// Stream returns a LogStream which streams from the log session.
func (ls *LogSession) Stream() (LogStream, error) {
	lch := make(chan buildlog.Line)
	if !ls.trySubscribe(lch) {
		return nil, ErrSessionClosed
	}
	return chanLogStream(lch), nil
}

// trySend tries to send a line to the channel and returns whether it was sucessful.
func trySend(line buildlog.Line, ch chan<- buildlog.Line) bool {
	defer func() { recover() }()
	ch <- line
	return true
}

// distributor starts the goroutine which distributes logs to clients.
func (ls *LogSession) distributor() {
	subch := make(chan chan<- buildlog.Line)
	ls.subscribe = subch
	ls.wg.Add(1)
	go func() {
		defer ls.wg.Done()
		defer close(subch)
		subscribers := []chan<- buildlog.Line{}
		log := []buildlog.Line{}
	f:
		for {
			select {
			case l, ok := <-ls.in: // handle incoming log line
				// if log input is closed, shutdown
				if !ok {
					break f
				}

				// add line to log
				log = append(log, l)

				// send line to subscribers
				for i := 0; i < len(subscribers); i++ {
					if !trySend(l, subscribers[i]) {
						// handle subscriber disconnect
						subscribers = subscribers[:i+copy(subscribers[i:], subscribers[i+1:])]
						i--
					}
				}
			case s := <-subch: // handle subscription
				// catch the subscriber up
				for _, l := range log {
					if !trySend(l, s) {
						// failed to catch them up
						continue
					}
				}

				// add subscribers
				subscribers = append(subscribers, s)
			}
		}

		// eject subscribers
		for _, s := range subscribers {
			close(s)
		}

		// save log to disk
		ls.err = ls.store.Save(ls.bi, log)
	}()
}

// Wait waits for the LogSession to close.
func (ls *LogSession) Wait() {
	ls.wg.Wait()
}

type logSessionLogHandler struct {
	ch chan<- buildlog.Line
	ls *LogSession
	lm *LogManager
}

func (lslh *logSessionLogHandler) Log(l buildlog.Line) error {
	lslh.ch <- l
	return nil
}

func (lslh *logSessionLogHandler) Close() error {
	// close input channel
	func() {
		defer func() { recover() }()
		close(lslh.ch)
	}()

	// deregister session
	lslh.lm.lck.Lock()
	defer lslh.lm.lck.Unlock()
	delete(lslh.lm.buildlookup, lslh.ls.bi.Hash)

	// wait for session shutdown
	lslh.ls.Wait()
	return lslh.ls.err
}

// LogManager manages logs.
type LogManager struct {
	lck         sync.Mutex
	store       *LogStore
	buildlookup map[[sha256.Size]byte]*LogSession
}

// Stream attempts to acquire a log stream.
func (lm *LogManager) Stream(bi buildmanager.BuildInfo) (LogStream, error) {
	// do locking
	lm.lck.Lock()
	defer lm.lck.Unlock()

	// look for a session
	sess := lm.buildlookup[bi.Hash]
	if sess != nil {
		ls, err := sess.Stream()
		if err != ErrSessionClosed {
			return ls, err
		}
		delete(lm.buildlookup, bi.Hash)
	}

	// pull off of disk
	return lm.store.Stream(bi)
}

// Log implements buildmanager.LogProvider.
func (lm *LogManager) Log(bi buildmanager.BuildInfo) (buildlog.Handler, error) {
	lm.lck.Lock()
	defer lm.lck.Unlock()

	// create session-handler pair
	lch := make(chan buildlog.Line)
	ls := &LogSession{
		in:    lch,
		store: lm.store,
		bi:    bi,
	}
	lh := &logSessionLogHandler{
		ch: lch,
		ls: ls,
		lm: lm,
	}

	// start distributor
	ls.distributor()

	// store session into manager
	lm.buildlookup[bi.Hash] = ls

	return lh, nil
}

// ServeHTTP implements http.Handler on the LogManager.
//
// The request takes the following query:
//
// 		buildhash: the hexidecimal sha256 hash of the build
//
// The request responds with HTML5 SSE with the following message types:
//
// 		start: indicates that event streaming has started (also sent on reconnect - starts from beginning)
//
// 		log: contains a buildlog.Line in data
//
// 		terminate: contains an error causing termination; EOF means that it finished streaming the log
func (lm *LogManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// decode request
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse form: %q", err.Error()), http.StatusBadRequest)
		return
	}
	hash, err := hex.DecodeString(r.FormValue("buildhash"))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to decode build hash: %q", err.Error()), http.StatusBadRequest)
		return
	}
	if len(hash) != sha256.Size {
		http.Error(w, fmt.Sprintf("Hash is %d bytes; expected %d bytes.", len(hash), sha256.Size), http.StatusBadRequest)
		return
	}
	var bi buildmanager.BuildInfo
	copy(bi.Hash[:], hash)

	// request LogStream
	ls, err := lm.Stream(bi)
	if os.IsNotExist(err) {
		http.Error(w, "Build not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to acquire log stream: %q", err.Error()), http.StatusInternalServerError)
		return
	}
	defer ls.Close()

	// start HTML5 Server Sent Events
	evs, err := sse.NewSender(w)
	if err != nil {
		return
	}

	// send log as events
	for {
		// get next log line
		l, err := ls.NextLine()

		// send termination
		if err != nil {
			// notify client of error
			evs.SendEvent(sse.Event{
				Name: "terminate",
				Data: err.Error(),
			})

			// disconnect
			return
		}

		// encode log line to JSON
		dat, _ := json.Marshal(l)

		// send line
		err = evs.SendEvent(sse.Event{
			Name: "log",
			Data: string(dat),
		})
		if err != nil {
			// connection error
			return
		}
	}
}
