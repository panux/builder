package main

import (
	"encoding/json"
	"log"
	"os"

	"gitlab.com/panux/builder/pkgen"
	"gitlab.com/panux/builder/pkgen/buildmanager"
	"golang.org/x/tools/godoc/vfs"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %q", err.Error())
	}
	g, _, err := (&buildmanager.Builder{
		SourceTree: vfs.OS(wd),
		Arch:       pkgen.SupportedArch,
	}).GetGraph()
	if err != nil {
		log.Fatalf("Failed to build: %q", err.Error())
	}

	graph := map[string][]string{}
	aj, err := g.GetJob("all")
	if err != nil {
		log.Fatalf("Failed to get all job: %q", err.Error())
	}
	ad, err := aj.Dependencies()
	if err != nil {
		log.Fatalf("Failed to get all job dependencies: %q", err.Error())
	}
	log.Println(ad)
	for _, v := range ad {
		j, err := g.GetJob(v)
		if err != nil {
			log.Printf("Failed to get job: %q", err.Error())
			continue
		}
		deps, err := j.Dependencies()
		if err != nil {
			log.Printf("Failed to get job deps: %q", err.Error())
			continue
		}
		graph[v] = deps
	}

	json.NewEncoder(os.Stdout).Encode(graph)
}
