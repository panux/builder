package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/panux/builder/pkgen"
	"github.com/panux/builder/pkgen/buildmanager"
)

// parseJobName parses the name of a job into identifiers for a build.
func parseJobName(jobname string) (name string, arch pkgen.Arch, bootstrap bool) {
	if strings.HasSuffix(jobname, "-bootstrap") {
		bootstrap = true
		jobname = strings.TrimSuffix(jobname, "-bootstrap")
	}
	spl := strings.Split(jobname, ":")
	name = spl[0]
	arch = pkgen.Arch(spl[1])
	return name, arch, bootstrap
}

// BranchStatus is the status of a branch.
type BranchStatus struct {
	lck sync.RWMutex

	// BranchName is the name of the branch.
	BranchName string `json:"branch"`

	// Builds is the set of BuildStatus objects.
	Builds map[string]*BuildStatus `json:"builds"`
}

func (bs *BranchStatus) infoCallback(jobname string, info buildmanager.BuildInfo) error {
	bs.lck.Lock()
	defer bs.lck.Unlock()

	bs.Builds[jobname].Info = &info
	return nil
}

func (bs *BranchStatus) updateState(job string, state BuildState) {
	bs.lck.Lock()
	defer bs.lck.Unlock()

	if bs.Builds[job] == nil {
		return
	}
	bs.Builds[job].State = state
}

// OnQueued is used to implement EventHandler.
func (bs *BranchStatus) OnQueued(job string) {
	bs.updateState(job, BuildStateQueued)
}

// OnStart is used to implement EventHandler.
func (bs *BranchStatus) OnStart(job string) {
	bs.updateState(job, BuildStateRunning)
}

// OnFinish is used to implement EventHandler.
func (bs *BranchStatus) OnFinish(job string) {
	bs.updateState(job, BuildStateFinished)
}

// OnError is used to implement EventHandler.
func (bs *BranchStatus) OnError(job string, err error) {
	log.Printf("Build %q failed: %q\n", job, err.Error())
	bs.updateState(job, BuildStateFailed)
}

// ListCallback can be used for listcallback in the Build func.
func (bs *BranchStatus) ListCallback(list []string) error {
	//generate build map
	builds := make(map[string]*BuildStatus)
	for _, b := range list {
		if b == "all" {
			continue
		}
		name, arch, bootstrap := parseJobName(b)
		builds[b] = &BuildStatus{
			Name:      name,
			Arch:      arch,
			Bootstrap: bootstrap,
			State:     BuildStateWaiting,
		}
	}

	//lock
	bs.lck.Lock()
	defer bs.lck.Unlock()

	//swap in builds
	bs.Builds = builds

	return nil
}

// ServeHTTP implements http.Handler on *BranchStatus.
// It serves the *BranchStatus as JSON after read-locking it.
func (bs *BranchStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bs.lck.RLock()
	defer bs.lck.RUnlock()

	json.NewEncoder(w).Encode(bs)
}

// BuildStatus is the status of a build.
type BuildStatus struct {
	Name      string                  `json:"name"`
	Arch      pkgen.Arch              `json:"arch"`
	Bootstrap bool                    `json:"bootstrap"`
	Info      *buildmanager.BuildInfo `json:"info,omitempty"`
	State     BuildState              `json:"state"`
}

// BuildState is a build state.
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
