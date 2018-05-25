//Package pkgen processes Panux .pkgen packaging files
package pkgen

import (
	"errors"
	"fmt"
	"runtime"
)

// ArchSet is a set of supported Arch's.
// A nil value indicates that all Arch's are supported.
type ArchSet []Arch

// Supports checks if the ArchSet supports arch.
func (a ArchSet) Supports(arch Arch) bool {
	if a == nil {
		return true
	}
	for _, v := range a {
		if v == arch {
			return true
		}
	}
	return false
}

// Arch is an architecture
type Arch string

func (a Arch) String() string {
	return string(a)
}

// Convert implements the makefile.Text interface
func (a Arch) Convert() string {
	if !a.Supported() {
		panic(fmt.Errorf("invalid arch %s", a.String()))
	}
	return a.String()
}

// AutoTools returns the name used by autotools for the Arch
func (a Arch) AutoTools() string {
	switch a {
	case Archx86: //autotools treats x86 as "i.86"
		return "i386"
	default:
		return a.String()
	}
}

// GoArch returns the name of an arch used by Go/Kubernetes
func (a Arch) GoArch() string {
	switch a {
	case Archx86:
		return "386"
	case Archx86_64:
		return "amd64"
	default:
		return a.String()
	}
}

// a86run is the set of CPU arch supported on x86_64 platforms
var a86run = []Arch{Archx86, Archx86_64}

// RunsOn returns what Arch's code from this arch will run on
// Why: 32-bit code can be built on 64-bit systems (e.g. x86 can build on x86_64)
func (a Arch) RunsOn() []Arch {
	switch a {
	case Archx86:
		return a86run
	default:
		return []Arch{a}
	}
}

// Supported checks if an Arch is recognized and will be processed correctly
func (a Arch) Supported() bool {
	switch a {
	case Archx86:
	case Archx86_64:
	default:
		return false
	}
	return true
}

// Arch constants
const (
	Archx86_64 Arch = "x86_64"
	Archx86    Arch = "x86"
)

// ErrUnsupportedArch is an error for an architecture that is not recognized
var ErrUnsupportedArch = errors.New("unsupported arch")

// GetHostArch returns the arch on the host system
func GetHostArch() (Arch, error) {
	switch runtime.GOARCH {
	case "amd64":
		return Archx86_64, nil
	case "i386":
		return Archx86, nil
	default:
		return "", ErrUnsupportedArch
	}
}

// SupportedArch is the set of supported Arch
var SupportedArch = ArchSet{"x86_64", "x86"}
