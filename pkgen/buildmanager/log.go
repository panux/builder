package buildmanager

import "gitlab.com/panux/builder/pkgen/buildlog"

// LogProvider is an interface for requesting log handlers for builds/
type LogProvider interface {
	// Log takes BuildInfo and returns a log handler.
	Log(BuildInfo) (buildlog.Handler, error)
}
