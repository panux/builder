package pkgen

import (
	"bytes"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// PackageGenerator is the preprocessed pkgen.
type PackageGenerator struct {
	// Package is a list of packages generated and their dependencies.
	// Required.
	Packages map[string]Package `json:"packages"`

	// Arch is a list of supported architectures.
	// Optional. Defualts to nil.
	Arch ArchSet `json:"arch"`

	// HostArch is the Arch which the package will be compiled on.
	// Required.
	HostArch Arch `json:"hostArch"`

	// BuildArch is the Arch which the package will be compiled for.
	// Required.
	BuildArch Arch `json:"buildArch"`

	// Version is the version of the package built.
	// Required.
	Version string `json:"version"`

	// Sources is a list of URLs for sources.
	// Optional.
	Sources []*url.URL `json:"sources,omitempty"`

	// Script is the script for building the package.
	// Optional.
	Script []string `json:"script,omitempty"`

	// BuildDependencies is a set of packages required for compilation.
	// Optional.
	BuildDependencies []string `json:"buildDependencies,omitempty"`

	// Builder is the builder to be used to compile the package.
	// Required.
	Builder Builder `json:"builder"`

	// Cross is whether or not the package may be cross compiled.
	// Not supported. Reserved for future use.
	Cross bool `json:"cross,omitempty"`

	// NoBootstrap is an option to force-unbootstrap a dependency.
	// Format: {"python":true}
	NoBootstrap map[string]bool `json:"nobootstrap"`
}

// Preprocess preprocesses a RawPackageGenerator into a PackageGenerator.
func (rpg *RawPackageGenerator) Preprocess(hostarch Arch, buildarch Arch, bootstrap bool) (*PackageGenerator, error) {
	pg := new(PackageGenerator)
	pg.Packages = make(map[string]Package)
	for n, p := range rpg.Packages {
		if p.Dependencies == nil {
			p.Dependencies = []string{}
		}
		pg.Packages[n] = p
	}
	pg.Arch = rpg.Arch
	pg.HostArch = hostarch
	pg.BuildArch = buildarch
	pg.Version = fmt.Sprintf("%s-%d", rpg.Version, rpg.Build)
	pg.Sources = make([]*url.URL, len(rpg.Sources))
	for i, v := range rpg.Sources {
		vpp, err := rpg.tmpl(fmt.Sprintf("src-%d", i), v, buildarch, hostarch)
		if err != nil {
			return nil, err
		}
		u, err := url.Parse(vpp)
		if err != nil {
			return nil, err
		}
		pg.Sources[i] = u
	}
	script, err := rpg.tmpl("script", strings.Join(rpg.Script, "\n"), buildarch, hostarch)
	if err != nil {
		return nil, err
	}
	pg.Script = strings.Split(script, "\n")
	pg.BuildDependencies = rpg.BuildDependencies
	pg.Builder, err = ParseBuilder(rpg.Builder)
	if err != nil {
		return nil, err
	}
	if pg.Builder.IsBootstrap() && !bootstrap {
		pg.Builder = BuilderDefault
	}
	pg.Cross = rpg.Cross
	pg.NoBootstrap = rpg.NoBootstrap
	return pg, nil
}

// tmpl preprocesses a value with text/template.
func (rpg *RawPackageGenerator) tmpl(name string, in string, buildarch Arch, hostarch Arch) (string, error) {
	var fnm template.FuncMap
	fnm = template.FuncMap{
		"extract": func(name string, ext string) string {
			return strings.Join(
				[]string{
					fmt.Sprintf("tar -xf src/%s-%s.tar.%s", name, rpg.Version, ext),
					fmt.Sprintf("mv %s-%s %s", name, rpg.Version, name),
				},
				"\n",
			)
		},
		"pkmv": func(file string, srcpkg string, destpkg string) string {
			if strings.HasSuffix(file, "/") { // cut off trailing /
				file = file[:len(file)-2]
			}
			dir, _ := filepath.Split(file)
			mv := fmt.Sprintf("mv %s %s",
				filepath.Join("out", srcpkg, file),
				filepath.Join("out", destpkg, dir),
			)
			if dir != "" {
				return strings.Join([]string{
					fmt.Sprintf("mkdir -p %s", filepath.Join("out", destpkg, dir)),
					mv,
				}, "\n")
			}
			return mv
		},
		"mvman": func(pkg string) string {
			return fmt.Sprintf("mkdir -p out/%s-man/usr/share\nmv out/%s/usr/share/man out/%s-man/usr/share/man", pkg, pkg, pkg)
		},
		"configure": func(dir string, args ...string) string {
			return fmt.Sprintf("(cd %s && ./configure %s --prefix=/usr --sysconfdir=/etc --mandir=/usr/share/man --localstatedir=/var %s)", dir, fnm["confflags"].(func() string)(), strings.Join(args, " "))
		},
		"confarch": func() string {
			return buildarch.AutoTools()
		},
		"hostarch": func() Arch {
			return hostarch
		},
		"buildarch": func() Arch {
			return buildarch
		},
		"confflags": func() string {
			return fmt.Sprintf("--build %s-pc-linux-musl --host %s-pc-linux-musl", buildarch.AutoTools(), hostarch.AutoTools())
		},
	}
	tmpl, err := template.New(name).Funcs(fnm).Parse(in)
	if err != nil {
		return "", err
	}
	buf := bytes.NewBuffer(nil)
	err = tmpl.Execute(buf, rpg)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ListPackages returns a sorted list of packages.
func (pg *PackageGenerator) ListPackages() []string {
	pkl := make([]string, len(pg.Packages))
	i := 0
	for n := range pg.Packages {
		pkl[i] = n
		i++
	}
	sort.Strings(pkl)
	return pkl
}
