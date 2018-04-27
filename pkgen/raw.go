package pkgen

import (
	"io"

	"gopkg.in/yaml.v2"
)

//RawPackageGenerator is the package generator in raw form (after YAML unmarshalling)
type RawPackageGenerator struct {
	Packages          map[string]Package     //list of packages generated
	Arch              ArchSet                //supported architectures (works on all if nil)
	Version           string                 //version of package
	Build             uint                   //build number (added to end of version)
	Sources           []string               //list of source URLs (raw)
	Script            []string               //script for building
	BuildDependencies []string               //build dependencies
	Builder           string                 //builder (bootstrap, docker, default)
	Cross             bool                   //Whether or not the package can be cross-compiled
	Data              map[string]interface{} //user-provided data
}

//Package is a package entry in a pkgen
type Package struct {
	Dependencies []string
}

//UnmarshalPkgen unmarshals a raw pkgen from YAML
func UnmarshalPkgen(r io.Reader) (*RawPackageGenerator, error) {
	rpg := new(RawPackageGenerator)
	err := yaml.NewDecoder(r).Decode(rpg)
	if err != nil {
		return nil, err
	}
	return rpg, nil
}
