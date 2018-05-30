package pkgen

import "errors"

// Builder is a package builder type
type Builder string

// Buiilder constants
const (
	BuilderDefault   Builder = "default"
	BuilderDocker    Builder = "docker"
	BuilderBootstrap Builder = "bootstrap"
)

// ErrUnsupportedBuilder is an error indicating that the Builder is not supported
var ErrUnsupportedBuilder = errors.New("builder not supported")

// ParseBuilder parses a Builder from a string.
// An empty string is interpreted as BuilderDefault.
func ParseBuilder(str string) (Builder, error) {
	switch Builder(str) {
	//supported builders
	case BuilderDefault:
	case BuilderDocker:
	case BuilderBootstrap:
	//default
	case "":
		return BuilderDefault, nil
	//deprecated builders
	case "alpine", "panux":
		return BuilderDefault, nil
	//unsupported builder
	default:
		return "", ErrUnsupportedBuilder
	}
	return Builder(str), nil
}

// IsBootstrap checks if the builder is BuilderBootstrap.
func (b Builder) IsBootstrap() bool {
	return b == BuilderBootstrap
}
