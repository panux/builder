package pkgen

import (
	"io"

	"gopkg.in/yaml.v2"
)

// RawPackageGenerator is the package generator in raw form (after YAML unmarshalling).
type RawPackageGenerator struct {
	// Packages is the list of packages generated by this pkgen.
	// Required.
	Packages map[string]Package

	// Arch is the set of supported architectures.
	// Optional.
	Arch ArchSet

	// Version is the version of the package.
	// Required.
	Version string

	// Build is the build number (added to end of version).
	// Optional.
	Build uint

	// Sources is a list of source URLs.
	// These will be preprocessed using "text/template".
	// Optional.
	Sources []string

	// Script is the script used for building the package.
	// This will be preprocessed using "text/template".
	// Required.
	Script []string

	// BuildDependencies is the set of build dependencies.
	// Required.
	BuildDependencies []string

	// Builder is the system used to build the pkgen.
	// Possible builders are: "bootstrap", "docker", or "default".
	// Optional. Defaults to "default".
	Builder string

	// Cross indicates whether or not the package can be cross-compiled.
	// Optional. Defaults to false.
	Cross bool

	// Data is a set of user-defined data.
	Data map[string]interface{}

	// NoBootstrap is an option to force-unbootstrap a dependency.
	// Format: {"python":true}
	NoBootstrap map[string]bool
}

// Package is a package entry in a pkgen.
type Package struct {
	// Dependencies is the set of dependencies the package will have.
	Dependencies []string
}

// UnmarshalPkgen unmarshals a raw pkgen from YAML.
func UnmarshalPkgen(r io.Reader) (*RawPackageGenerator, error) {
	rpg := new(RawPackageGenerator)
	err := yaml.NewDecoder(r).Decode(rpg)
	if err != nil {
		return nil, err
	}
	if rpg.NoBootstrap == nil {
		rpg.NoBootstrap = map[string]bool{}
	}
	return rpg, nil
}
