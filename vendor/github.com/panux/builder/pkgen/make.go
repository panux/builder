package pkgen

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	makefile "github.com/panux/go-makefile"
)

// MakeVars is a set of makefile.MakeVar to use for generated makefiles.
type MakeVars struct {
	SrcTar    makefile.MakeVar //variable with path to source tarball
	TarOut    makefile.MakeVar //variable with path to the tar output directory
	HostArch  makefile.MakeVar //variable with host arch
	BuildArch makefile.MakeVar //variable with build arch
}

// InitializeVars adds variable initialization of MakeVars to a Makefile.
func (pg *PackageGenerator) InitializeVars(mv MakeVars, b *makefile.Builder) {
	//put notice
	b.Comment().
		Line("Section autogenerated by PackageGenerator.InitializeVars (see godoc.org/github.com/panux/builder/pkgen)").
		Line("DO NOT EDIT")

	//initialize variables
	b.SetVar(mv.SrcTar, makefile.FilePath("src.tar"))
	b.SetVar(mv.TarOut, makefile.FilePath("tars"))
	b.SetVar(mv.HostArch, pg.HostArch)
	b.SetVar(mv.BuildArch, pg.BuildArch)
}

// DefaultVars is the default MakeVars.
var DefaultVars = MakeVars{
	SrcTar:    "SRCTAR",
	TarOut:    "TAROUT",
	HostArch:  "HOSTARCH",
	BuildArch: "BUILDARCH",
}

// dirRule creates a Makefile rule for creating a directory.
func dirRule(b *makefile.Builder, dirname makefile.Text) *makefile.Rule {
	r := b.NewRule(dirname).
		Print(makefile.JoinText(" ",
			makefile.RawText("MKDIR"),
			dirname,
		))
	r.NewCmd("mkdir").
		AddArg(dirname).
		SetNoPrint()
	return r
}

// GenMakeInfoComment generates a Makefile comment with pretty-printed info.
func (pg *PackageGenerator) GenMakeInfoComment(b *makefile.Builder) {
	infotitle := "Package Information"
	def := func(v string, d string) string {
		if v == "" {
			return d
		}
		return v
	}
	b.Comment().
		Line(infotitle).Line(strings.Repeat("-", len(infotitle))). //title
		Line(fmt.Sprintf("Packages: %s", strings.Join(pg.ListPackages(), " "))).
		Line(fmt.Sprintf("Version: %s", pg.Version)).
		Line(fmt.Sprintf("Arch (host, build): %s, %s", pg.HostArch.String(), pg.BuildArch.String())).
		Line(fmt.Sprintf("Builder: %s", pg.Builder)).
		Line(fmt.Sprintf("Build Dependencies: %s", def(strings.Join(pg.BuildDependencies, " "), "None")))
}

// GenMake adds the PackageGenerator script to a Makefile.
func (pg *PackageGenerator) GenMake(mv MakeVars, b *makefile.Builder) {
	//put notice
	b.Comment().
		Line("Section autogenerated by PackageGenerator.GenMake (see godoc.org/github.com/panux/builder/pkgen)").
		Line("DO NOT EDIT")

	//misc
	st := makefile.RawText("script")
	ot := makefile.FilePath("out")
	srct := makefile.FilePath("src")
	uts := makefile.RawText("untarsource")

	//sections
	basics := b.SectionBuilder()
	dirsec := b.SectionBuilder()
	dirsec.Comment().Line("Directory structure rules")

	//basics
	trule := basics.NewRule(makefile.RawText("gentars")).Phony()
	pkginfos := basics.NewRule(makefile.RawText("pkginfos")).Phony()
	defer basics.AppendPhony()

	//put basic directory structure
	dirRule(dirsec, srct)
	dirRule(dirsec, ot)
	dirRule(dirsec, mv.TarOut.Sub())

	//do stuff
	for _, p := range pg.ListPackages() {
		//out directory rule
		odir := makefile.FilePath(path.Join("out", p))
		dirRule(dirsec, odir).AddDep(ot)

		//add pkginfo rule
		pkin := makefile.FilePath(filepath.Join("out", p, ".pkginfo"))
		b.NewRule(pkin).
			AddDep(
				makefile.FilePath(
					filepath.Join(
						"src",
						fmt.Sprintf("%s.pkginfo", p),
					),
				),
			).
			AddDep(odir).
			Print(makefile.JoinText(" ",
				makefile.RawText("PKGINFO"),
				makefile.FilePath(p),
			)).
			NewCmd("cp").AddArg(makefile.RawText("-f")).
			AddArg(makefile.Dep1).
			AddArg(makefile.Target).
			SetNoPrint()
		pkginfos.AddDep(pkin)

		//add output tarring rule
		tname := makefile.FilePath(fmt.Sprintf("%s.tar.gz", p))
		trname := makefile.JoinText("/",
			mv.TarOut.Sub(),
			tname,
		)
		b.NewRule(trname).
			AddDep(st).
			AddDep(mv.TarOut.Sub()).
			Print(makefile.JoinText(" ",
				makefile.RawText("TAR"),
				tname,
			)).
			NewCmd("tar").
			AddArg(makefile.RawText("-cf")).
			AddArg(makefile.Target).
			AddArg(makefile.RawText("-C")).
			AddArg(makefile.FilePath(filepath.Join("out", p))).
			AddArg(makefile.RawText(".")).
			SetNoPrint()
		trule.AddDep(trname)
	}

	//add rule for source pkginfos
	basics.NewRule(makefile.JoinText("/",
		srct,
		makefile.ExtPattern("pkginfo"),
	)).AddDep(uts)

	//add rule to un-tar source
	basics.NewRule(uts).
		AddDep(mv.SrcTar.Sub()).AddDep(srct).
		NewCmd("tar").
		AddArg(makefile.RawText("-xf")).AddArg(makefile.Dep1).
		AddArg(makefile.RawText("-C")).AddArg(srct)

	//add script rule
	sr := b.NewRule(st).OneShell().AddDep(uts).AddDep(makefile.RawText("pkginfos"))
	sr.NewCmd("set -ex")
	for _, l := range pg.Script {
		sr.NewCmd(l)
	}

	//add pkgs.tar rule
	b.NewRule(makefile.FilePath("pkgs.tar")).
		AddDep(makefile.RawText("gentars")).
		NewCmd("tar").
		AddArg(makefile.RawText("-cf")).
		AddArg(makefile.RawText("pkgs.tar")).
		AddArg(makefile.RawText("-C")).
		AddArg(mv.TarOut.Sub()).
		AddArg(makefile.RawText("."))
}

// GenFullMakefile creates an entire Makefile.
func (pg *PackageGenerator) GenFullMakefile(mv MakeVars) *makefile.Builder {
	b := makefile.NewBuilder()
	//put notice
	b.Comment().
		Line("Makefile autogenerated by PackageGenerator.GenFullMakefile (see godoc.org/github.com/panux/builder/pkgen)").
		Line("DO NOT EDIT")
	b.BlankLine()

	//add info comment
	pg.GenMakeInfoComment(b)
	b.BlankLine()

	//add "all" rule
	b.NewRule(makefile.RawText("all")).AddDep(makefile.RawText("gentars"))
	b.BlankLine()

	//sections
	varsection := b.SectionBuilder()
	b.BlankLine()
	buildsection := b.SectionBuilder()
	b.BlankLine()

	//initialize variables
	pg.InitializeVars(mv, varsection)

	//create build rules
	pg.GenMake(mv, buildsection)
	return b
}