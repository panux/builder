package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/panux/builder/pkgen"
	"github.com/urfave/cli"
)

var prettyInfo = `
{{- range .}}{{with (preprocess (unmarshal (fopen .))) -}}
Packages:{{range .ListPackages}} {{.}}{{end}}
Version: {{.Version}}
Builder: {{.Builder}}
Build Dependencies:{{range .BuildDependencies}} {{.}}{{else}} None{{end}}
Cross Compilation Support: {{if .Cross}}Yes{{else}}No{{end}}
Sources:{{range .Sources}}
{{indent 4 .String}}{{else}} None{{end}}

{{end}}{{end -}}
`

func main() {
	app := cli.NewApp()
	app.Version = "3.0"
	app.Description = "A command to build pkgens"
	harch, err := pkgen.GetHostArch()
	if err != nil {
		panic(err)
	}
	app.Commands = []cli.Command{
		cli.Command{
			Name:  "info",
			Usage: "print info about pkgen",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "template",
					Value: prettyInfo,
					Usage: "template to use for info, args passed as .",
				},
				cli.StringFlag{
					Name:  "hostarch",
					Value: harch.String(),
					Usage: "host arch to use when preprocessing",
				},
				cli.StringFlag{
					Name:  "buildarch",
					Value: harch.String(),
					Usage: "build arch to use when preprocessing",
				},
			},
			Action: func(ctx *cli.Context) error {
				tmpl, err := template.New("info").
					Funcs(sprig.TxtFuncMap()).
					Funcs(
						template.FuncMap{
							"fopen": func(path string) (*os.File, error) {
								return os.Open(path)
							},
							"unmarshal": func(r io.Reader) (*pkgen.RawPackageGenerator, error) {
								return pkgen.UnmarshalPkgen(r)
							},
							"arch": func(name string) (a pkgen.Arch, err error) {
								a = pkgen.Arch(name)
								if !a.Supported() {
									err = fmt.Errorf("unsupported arch %q", name)
								}
								return
							},
							"preprocess": func(rpg *pkgen.RawPackageGenerator, archs ...pkgen.Arch) (*pkgen.PackageGenerator, error) {
								switch len(archs) {
								case 0:
									archs = []pkgen.Arch{
										pkgen.Arch(ctx.String("hostarch")),
										pkgen.Arch(ctx.String("buildarch")),
									}
								case 1:
									archs = []pkgen.Arch{archs[0], archs[0]}
								case 2:
								default:
									return nil, errors.New("too many arguments")
								}
								return rpg.Preprocess(archs[0], archs[1])
							},
						},
					).
					Parse(ctx.String("template"))
				if err != nil {
					return err
				}
				return tmpl.Execute(ctx.App.Writer, []string(ctx.Args()))
			},
		},
		cli.Command{
			Name:  "makefile",
			Usage: "generate a makefile for building the packages",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "makefile, f",
					Value: "Makefile",
					Usage: "file to write Makefile to",
				},
				cli.StringFlag{
					Name:  "hostarch",
					Value: harch.String(),
					Usage: "host arch to use when preprocessing",
				},
				cli.StringFlag{
					Name:  "buildarch",
					Value: harch.String(),
					Usage: "build arch to use when preprocessing",
				},
			},
			Action: func(ctx *cli.Context) (err error) {
				if len(ctx.Args()) != 1 {
					return cli.NewExitError("too many arguments", 65)
				}
				f, err := os.OpenFile(ctx.String("makefile"), os.O_CREATE|os.O_WRONLY, 0600)
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				defer func() {
					cerr := f.Close()
					if cerr != nil {
						if err == nil {
							err = cli.NewExitError(cerr, 65)
						}
					}
				}()
				inf, err := os.Open(ctx.Args()[0])
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				defer inf.Close()
				rpg, err := pkgen.UnmarshalPkgen(inf)
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				pg, err := rpg.Preprocess(
					pkgen.Arch(ctx.String("hostarch")),
					pkgen.Arch(ctx.String("buildarch")),
				)
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				_, err = pg.GenFullMakefile(pkgen.DefaultVars).WriteTo(f)
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				return nil
			},
		},
	}
	err = app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}
