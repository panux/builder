package main

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"text/template"

	"github.com/Masterminds/sprig"
	"github.com/urfave/cli"
	"gitlab.com/panux/builder/pkgen"
	makefile "gitlab.com/panux/go-makefile"
	"golang.org/x/tools/godoc/vfs"
)

var prettyInfo = `
{{- range .}}{{with (preprocess (unmarshal (fopen .)) false) -}}
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
	cctx, cancel := context.WithCancel(context.Background())
	sigch := make(chan os.Signal, 1)
	go func() {
		<-sigch
		log.Println("Cancelled")
		cancel()
	}()
	signal.Notify(sigch, syscall.SIGTERM)
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
							"preprocess": func(rpg *pkgen.RawPackageGenerator, bootstrap bool, archs ...pkgen.Arch) (*pkgen.PackageGenerator, error) {
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
								return rpg.Preprocess(archs[0], archs[1], bootstrap)
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
				cli.BoolFlag{
					Name:  "bootstrap",
					Usage: "whether to create a bootstrap makefile",
				},
			},
			Action: func(ctx *cli.Context) (err error) {
				if len(ctx.Args()) != 1 {
					return cli.NewExitError("wrong number of arguments", 65)
				}
				f, err := os.OpenFile(ctx.String("makefile"), os.O_CREATE|os.O_WRONLY, 0600)
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				go func() { //do cancel w/ file f
					<-cctx.Done()
					f.Close()
				}()
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
				go func() { //do cancel w/ inf
					<-cctx.Done()
					inf.Close()
				}()
				rpg, err := pkgen.UnmarshalPkgen(inf)
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				pg, err := rpg.Preprocess(
					pkgen.Arch(ctx.String("hostarch")),
					pkgen.Arch(ctx.String("buildarch")),
					ctx.Bool("bootstrap"),
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
		cli.Command{
			Name:  "src",
			Usage: "downloads all source files (and Makefile) into a tar",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "tar, f",
					Value: "src.tar",
					Usage: "tar file to write to (supports gz if extension present)",
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
				cli.UintFlag{
					Name:  "maxbuf",
					Value: 100 * 1024 * 1024,
					Usage: "maximum amount of data to buffer in bytes",
				},
				cli.BoolFlag{
					Name:  "bootstrap",
					Usage: "whether to create a bootstrap makefile",
				},
			},
			Action: func(ctx *cli.Context) (err error) {
				//pre-checks
				if len(ctx.Args()) != 1 {
					return cli.NewExitError("wrong number of arguments", 65)
				}
				ext := filepath.Ext(ctx.String("tar"))
				switch ext {
				case ".tar":
				case ".gz":
				default:
					return cli.NewExitError(fmt.Errorf("Unsupported extension %q in %q", ext, ctx.String("tar")), 65)
				}
				//load & preprocess pkgen
				inf, err := os.Open(ctx.Args()[0])
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				defer inf.Close()
				go func() { //do cancel w/ inf
					<-cctx.Done()
					inf.Close()
				}()
				rpg, err := pkgen.UnmarshalPkgen(inf)
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				pg, err := rpg.Preprocess(
					pkgen.Arch(ctx.String("hostarch")),
					pkgen.Arch(ctx.String("buildarch")),
					ctx.Bool("bootstrap"),
				)
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				//prep writer for tar
				tf, err := os.OpenFile(ctx.String("tar"), os.O_CREATE|os.O_WRONLY, 0600)
				if err != nil {
					return
				}
				var w io.WriteCloser
				switch ext {
				case ".tar":
					w = tf
				case ".gz":
					w = gzip.NewWriter(tf)
				}
				defer func() {
					cerr := w.Close()
					if cerr != nil {
						cerr = cli.NewExitError(cerr, 65)
						if err == nil {
							err = cerr
						} else {
							err = cli.NewMultiError(err, cerr)
						}
					}
				}()
				l, err := pkgen.NewMultiLoader(
					pkgen.NewHTTPLoader(
						http.DefaultClient,
						ctx.Uint("maxbuf"),
					),
					pkgen.NewFileLoader(
						vfs.OS(
							filepath.Dir(
								ctx.Args()[0],
							),
						),
					),
				)
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				//generate tar
				err = pg.WriteSourceTar(cctx, w, l, ctx.Uint("maxbuf"))
				return
			},
		},
		cli.Command{
			Name:  "build",
			Usage: "build a package",
			Flags: []cli.Flag{
				cli.UintFlag{
					Name:  "j",
					Value: 6,
					Usage: "number of jobs run concurrently",
				},
				cli.StringFlag{
					Name:  "pkgen, f",
					Value: "pkgen.yaml",
					Usage: "pkgen file to use",
				},
			},
			Action: func(ctx *cli.Context) (err error) {
				if len(ctx.Args()) != 0 {
					return cli.NewExitError("wrong number of arguments", 65)
				}
				self, err := exec.LookPath(os.Args[0])
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				makebin, err := exec.LookPath("make")
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				pkv := makefile.MakeVar("PKGEN")
				infv := makefile.MakeVar("INFILE")
				b := makefile.NewBuilder()
				b.Comment().
					Line("AUTOGENERATED DO NOT EDIT").
					Line("pre-load makefile for `pkgen build`")
				b.BlankLine()
				varsec := b.SectionBuilder()
				varsec.Comment().Line("Config Variables")
				varsec.SetVar(pkv, makefile.FilePath(self)).
					SetVar(infv, makefile.FilePath(ctx.String("pkgen")))
				b.BlankLine()
				srctn := makefile.RawText("src.tar")
				mfn := makefile.RawText("Makefile")
				b.NewRule(makefile.RawText("all")).
					AddDep(mfn)
				b.NewRule(mfn). //extract Makefile from source tar
						AddDep(srctn).
						NewCmd("tar -xf").
						AddArg(makefile.Dep1).
						AddArg(makefile.Target)
				b.NewRule(srctn).
					AddCmd(
						makefile.NewCmd(pkv.Sub()).
							AddArg(makefile.RawText("src")).
							AddArg(infv.Sub()),
					)
				err = func() (err error) {
					mf, err := os.OpenFile("Makefile", os.O_CREATE|os.O_WRONLY, 0600)
					if err != nil {
						return
					}
					defer func() {
						cerr := mf.Close()
						if err == nil {
							err = cerr
						}
					}()
					_, err = b.WriteTo(mf)
					return
				}()
				if err != nil {
					return cli.NewExitError(err, 65)
				}
				err = syscall.Exec(makebin,
					[]string{
						makebin,
						"-l",
						strconv.FormatUint(uint64(ctx.Uint("j")), 10),
					},
					os.Environ(),
				)
				return cli.NewExitError(err, 65)
			},
		},
	}
	err = app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}
