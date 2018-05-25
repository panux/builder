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

// PackageGenerator is the preprocessed pkgen
type PackageGenerator struct {
	Packages          map[string]Package `json:"packages"` //list of packages generated
	Arch              ArchSet            `json:"arch"`     //supported architectures (any means not sys-dependent, all means it will run on all)
	HostArch          Arch               `json:"hostArch"` //selected host and build arch
	BuildArch         Arch               `json:"buildArch"`
	Version           string             `json:"version"`                     //version of package (pre-processed)
	Sources           []*url.URL         `json:"sources,omitempty"`           //list of source URLs
	Script            []string           `json:"script,omitempty"`            //script for building
	BuildDependencies []string           `json:"buildDependencies,omitempty"` //build dependencies
	Builder           string             `json:"builder"`                     //builder (bootstrap, docker, default)
	Cross             bool               `json:"cross,omitempty"`             //whether or not the package can be cross-compiled
}

// Preprocess preprocesses a RawPackageGenerator into a PackageGenerator
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
	if rpg.Builder != "" {
		switch rpg.Builder {
		case "bootstrap":
			if !bootstrap {
				rpg.Builder = "default"
			}
		case "docker":
		case "default":
		case "panux":
			rpg.Builder = "default"
		default:
			return nil, fmt.Errorf("pkgen: invalid builder %q", rpg.Builder)
		}
		pg.Builder = rpg.Builder
	} else {
		pg.Builder = "default"
	}
	pg.Cross = rpg.Cross
	return pg, nil
}

// tmpl preprocesses a value with text/template
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
			if strings.HasSuffix(file, "/") { //cut off trailing /
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
		"hostarch": func() string {
			return hostarch.String()
		},
		"buildarch": func() string {
			return buildarch.String()
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

// ListPackages returns a sorted list of packages
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
