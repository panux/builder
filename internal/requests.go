// Package internal is used internally by worker (DO NOT USE EXTERNALLY)
package internal

import "github.com/panux/builder/pkgen"

// MkdirRequest is a request to make a dir on the worker
type MkdirRequest struct {
	// dir to make
	Dir string `json:"dir"`

	// whether to make parent dirs
	Parent bool `json:"parent,omitempty"`
}

// FileWriteRequest is a request to write a file (use POST with multipart form writer)
type FileWriteRequest struct {
	// path to write to
	Path string `json:"path"`
}

// FileReadRequest is a request to read from a file
type FileReadRequest struct {
	// path to read from
	Path string `json:"path"`
}

// CommandRequest is a request to execute a command
type CommandRequest struct {
	// Argv is the argument set for the command (required)
	Argv []string `json:"argv"`

	// Env is a set of environment variables to set (optional)
	Env map[string]string `json:"env,omitempty"`

	// If EnableStdin is true then stdin will be forwarded from the websocket
	EnableStdin bool `json:"stdin,omitempty"`
	// If DisableStdout is true then stdout will not be logged
	DisableStdout bool `json:"stdout,omitempty"`
	// If DisableStderr is true then stderr will not be logged
	DisableStderr bool `json:"stdout,omitempty"`
}

// BuildRequest is a request to build a package
type BuildRequest struct {
	// Pkgen is the package generator that will be used for the build
	Pkgen *pkgen.PackageGenerator `json:"pkgen"`
}
