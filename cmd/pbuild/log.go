package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/panux/builder/pkgen/buildlog"
	"github.com/panux/builder/pkgen/buildmanager"
)

// LogStream is an interface for a log reading mechanism
type LogStream interface {
	// NextLine gets the next line in the log.
	// If this is the end of the log, io.EOF will be returned.
	NextLine() (buildlog.Line, error)

	// Close closes the LogStream.
	Close()
}

// LogStore is a storage directory of logs
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

// LogSession is a logging system for one build
type LogSession struct {
	wg        sync.WaitGroup
	in        <-chan buildlog.Line
	subscribe chan chan<- buildlog.Line
	store     *LogStore
	bi        buildmanager.BuildInfo
	err       error
}

type chanLogStream chan buildlog.Line

func (ch chanLogStream) NextLine() (buildlog.Line, error) {
	line, ok := <-ch
	if !ok {
		return buildlog.Line{}, io.EOF
	}
	return line, nil
}

func (ch chanLogStream) Close() {
	close(ch)
}

func (ls *LogSession) trySubscribe(lch chan<- buildlog.Line) bool {
	defer recover()
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

//trySend tries to send a line to the channel and returns whether it was sucessful
func trySend(line buildlog.Line, ch chan<- buildlog.Line) bool {
	defer recover()
	ch <- line
	return true
}

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
			case l, ok := <-ls.in: //handle incoming log line
				//if log input is closed, shutdown
				if !ok {
					break f
				}

				//add line to log
				log = append(log, l)

				//send line to subscribers
				for i := 0; i < len(subscribers); i++ {
					if !trySend(l, subscribers[i]) {
						//handle subscriber disconnect
						subscribers = subscribers[:i+copy(subscribers[i:], subscribers[i+1:])]
						i--
					}
				}
			case s := <-subch: //handle subscription
				//catch the subscriber up
				for _, l := range log {
					if !trySend(l, s) {
						//failed to catch them up
						continue
					}
				}

				//add subscribers
				subscribers = append(subscribers, s)
			}
		}

		//eject subscribers
		for _, s := range subscribers {
			close(s)
		}

		//save log to disk
		ls.err = ls.store.Save(ls.bi, log)
	}()
}
