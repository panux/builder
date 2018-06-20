// Package internal is used internally by worker (DO NOT USE EXTERNALLY)
package internal

import "gitlab.com/panux/builder/pkgen"

// MkdirRequest is a request to make a dir on the worker.
type MkdirRequest struct {
	// Dir is the directory to make.
	// Required.
	Dir string `json:"dir"`

	// Parent is an option indicating whether or not to create parent directories.
	// Optional.
	Parent bool `json:"parent,omitempty"`
}

// FileWriteRequest is a request to write a file (use POST with multipart form writer).
type FileWriteRequest struct {
	// Path is the path to write the file to.
	// Required.
	Path string `json:"path"`
}

// FileReadRequest is a request to read from a file.
type FileReadRequest struct {
	// Path is the path to read the file from.
	// Required.
	Path string `json:"path"`
}

// CommandRequest is a request to execute a command.
type CommandRequest struct {
	// Argv is the argument set for the command.
	// Required.
	Argv []string `json:"argv"`

	// Env is a set of environment variables to set.
	// Optional.
	Env map[string]string `json:"env,omitempty"`

	// If EnableStdin is true then stdin will be forwarded from the websocket.
	// Optional.
	EnableStdin bool `json:"stdin,omitempty"`
	// If DisableStdout is true then stdout will not be logged.
	// Optional.
	DisableStdout bool `json:"stdout,omitempty"`
	// If DisableStderr is true then stderr will not be logged.
	// Optional.
	DisableStderr bool `json:"stdout,omitempty"`
}

// BuildRequest is a request to build a package.
type BuildRequest struct {
	// Pkgen is the package generator that will be used for the build.
	// Required.
	Pkgen *pkgen.PackageGenerator `json:"pkgen"`
}
