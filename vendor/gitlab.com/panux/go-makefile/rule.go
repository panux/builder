package makefile

import (
	"fmt"
	"sort"
	"strings"
)

//Rule is a Makefile rule
type Rule struct {
	Name    Text
	Deps    []Text
	Command []*Command
	Attr    RuleAttributes
}

//AddDep adds a dependency to the Rule
func (r *Rule) AddDep(dep Text) *Rule {
	if r.Deps == nil {
		r.Deps = []Text{dep}
	} else {
		r.Deps = append(r.Deps, dep)
	}
	return r
}

//AddCmd adds an existing Command to a rule
func (r *Rule) AddCmd(c *Command) *Rule {
	if r.Command == nil {
		r.Command = []*Command{c}
	} else {
		r.Command = append(r.Command, c)
	}
	return r
}

//NewCmd creates a new Command and adds it to the rule
func (r *Rule) NewCmd(base string) *Command {
	c := NewCmd(RawText(base))
	r.AddCmd(c)
	return c
}

//RuleAttributes is a struct containing the attributes of a Rule
type RuleAttributes struct {
	OneShell bool //goes in front of rule as .ONESHELL:
	Phony    bool
	Shell    Text //shell to use, goes in deps as SHELL:=path
}

//Phony configures a Rule to be phony
func (r *Rule) Phony() *Rule {
	r.Attr.Phony = true
	return r
}

//OneShell configures a Rule to be built with one shell
func (r *Rule) OneShell() *Rule {
	r.Attr.OneShell = true
	return r
}

//SetShell configures a Rule to use the specified shell
func (r *Rule) SetShell(sh Text) *Rule {
	r.Attr.Shell = sh
	return r
}

//Print prunts a piece of Text
func (r *Rule) Print(txt Text) *Rule {
	r.NewCmd("echo").AddArg(txt).SetNoPrint()
	return r
}

//Command is a command in a rule/shell substitution
type Command struct {
	Base    Text //base is the program to invoke (e.g. gcc)
	Args    []Text
	Env     map[ShellVar]Text
	NoPrint bool
}

func (c *Command) envString() string {
	estrs := make([]string, len(c.Env))
	i := 0
	for n, v := range c.Env {
		err := n.CheckValid()
		if err != nil {
			panic(err)
		}
		estrs[i] = fmt.Sprintf("%s=%s", n, v.Convert())
		i++
	}
	sort.Strings(estrs)
	return strings.Join(estrs, " ")
}

//String returns the command as a string
//NOTE: may panic
func (c *Command) String() string {
	str := c.Base.Convert()
	if len(c.Env) != 0 {
		str = c.envString() + " " + str
	}
	if len(c.Args) != 0 {
		astrs := make([]string, len(c.Args))
		for i, v := range c.Args {
			astrs[i] = v.Convert()
		}
		str = str + " " + strings.Join(astrs, " ")
	}
	if c.NoPrint {
		str = "@" + str
	}
	return str
}

//NewCmd returns a command using base as the Base and args as arguments
func NewCmd(base Text) *Command {
	return &Command{Base: base}
}

//AddArg adds an arg to a command
func (c *Command) AddArg(arg Text) *Command {
	if c.Args == nil {
		c.Args = []Text{arg}
	} else {
		c.Args = append(c.Args, arg)
	}
	return c
}

//AddArgSlice adds a slice of arguments to a command
func (c *Command) AddArgSlice(args []Text) *Command {
	if c.Args == nil {
		c.Args = args
	} else {
		c.Args = append(c.Args, args...)
	}
	return c
}

//SetNoPrint sets the NoPrint to true on a command
func (c *Command) SetNoPrint() *Command {
	c.NoPrint = true
	return c
}

//SetEnv sets an environment variable for the Command
func (c *Command) SetEnv(v ShellVar, val Text) *Command {
	if c.Env == nil {
		c.Env = make(map[ShellVar]Text)
	}
	c.Env[v] = val
	return c
}

//Sub returns a shell substitution (Text) which substitutes in this command
func (c *Command) Sub() ShellSub {
	return ShellSub{Cmd: c}
}

//ShellSub substitutes the output of a command
type ShellSub struct {
	Cmd *Command
}

//Convert formats the ShellSub as a shell substitution
func (ss ShellSub) Convert() string {
	return fmt.Sprintf("$(shell %s)", strings.TrimPrefix(ss.Cmd.String(), "@"))
}
