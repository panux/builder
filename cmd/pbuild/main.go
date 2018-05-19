package main

import "github.com/panux/builder/pkgen"

func main() {}

// Config is the configuration struct
var Config struct {
	HTTPAddr        string              `json:"http"`
	Branches        []string            `json:"branches"`
	Static          string              `json:"static"`
	LogDir          string              `json:"logs"`
	BuildManager    string              `json:"manager"`
	BuildManagerKey string              `json:"managerKey"`
	CacheDir        string              `json:"cache"`
	OutputDir       string              `json:"output"`
	GitDir          string              `json:"gitDir"`
	GitRepo         string              `json:"gitRepo"`
	Arch            pkgen.ArchSet       `json:"arch"`
	Parallel        map[pkgen.Arch]uint `json:"parallel"`
}
