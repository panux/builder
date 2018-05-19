package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/panux/builder/pkgen"
	"github.com/panux/builder/pkgen/buildmanager"
)

// BranchStatus is the status of a branch
type BranchStatus struct {
	lck sync.RWMutex

	// BranchName is the name of the branch
	BranchName string `json:"branch"`

	// Builds is the set of BuildStatus objects
	Builds map[string]map[pkgen.Arch]*BuildStatus `json:"builds"`
}

// ServeHTTP implements http.Handler on *BranchStatus.
// It serves the *BranchStatus as JSON after read-locking it.
func (b *BranchStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.lck.RLock()
	defer b.lck.RUnlock()
	json.NewEncoder(w).Encode(b)
}

// BuildStatus is the status of a build
type BuildStatus struct {
	Name  string                  `json:"name"`
	Arch  pkgen.Arch              `json:"arch"`
	Info  *buildmanager.BuildInfo `json:"info,omitempty"`
	State BuildState              `json:"state"`
}

// BuildState is a build state
type BuildState string

const (
	// BuildStateWaiting is a BuildState indicating that the build has not yet been queued.
	BuildStateWaiting BuildState = "waiting"
	//BuildStateQueued is a BuildState indicating that the build has been queued.
	BuildStateQueued BuildState = "queued"
	//BuildStateRunning is a BuildState indicating that the build is running.
	BuildStateRunning BuildState = "running"
	//BuildStateFinished is a BuildState indicating that the build has finished.
	BuildStateFinished BuildState = "finished"
	//BuildStateFailed is a BuildState indicating that the build has failed.
	BuildStateFailed BuildState = "failed"
)
