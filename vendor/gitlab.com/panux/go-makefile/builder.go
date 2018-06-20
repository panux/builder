package makefile

import (
	"bufio"
	"io"
)

//Builder is a tool to build a Makefile in memory
type Builder struct {
	entries []interface{}
}

func (b *Builder) addSomething(smth interface{}) {
	if b.entries == nil {
		b.entries = []interface{}{smth}
	} else {
		b.entries = append(b.entries, smth)
	}
}

//NewRule creates a new Rule and adds it to the Makefile
func (b *Builder) NewRule(target Text) *Rule {
	r := new(Rule)
	r.Name = target
	b.addSomething(r)
	return r
}

//AddRaw adds a raw string to a Makefile
//this method does not handle newlines - please add a newline before the text
func (b *Builder) AddRaw(str string) *Builder {
	b.addSomething(RawText(str))
	return b
}

//BlankLine adds a blank line to a Makefile
func (b *Builder) BlankLine() *Builder {
	return b.AddRaw("\n")
}

//Comment returns a new Comment
func (b *Builder) Comment() *Comment {
	c := &Comment{}
	b.addSomething(c)
	return c
}

//SetVarSlice sets a MakeVar to a slice of values
func (b *Builder) SetVarSlice(mv MakeVar, vals []Text) *Builder {
	b.addSomething(makeVarAssign{v: mv, val: vals, op: "="})
	return b
}

//SetVar sets a variable to a single value
func (b *Builder) SetVar(mv MakeVar, val Text) *Builder {
	return b.SetVarSlice(mv, []Text{val})
}

//AppendVar appends a value to a variable
func (b *Builder) AppendVar(mv MakeVar, val Text) *Builder {
	b.addSomething(makeVarAssign{v: mv, val: []Text{val}, op: "+="})
	return b
}

//AppendPhony appends a .PHONY rule using all of the phony rules that have been appended previously
func (b *Builder) AppendPhony() *Builder {
	pr := b.NewRule(RawText(".PHONY"))
	for _, v := range b.entries {
		r, ok := v.(*Rule)
		if ok {
			if r.Attr.Phony {
				pr.AddDep(r.Name)
			}
		}
	}
	return b
}

//SectionBuilder returns another Builder which will be substituted in
func (b *Builder) SectionBuilder() *Builder {
	sb := NewBuilder()
	b.addSomething(sb)
	return sb
}

//WriteTo writes the Makefile to an io.Writer
func (b *Builder) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	mw := makeWriter{bufio.NewWriter(cw)}
	for _, v := range b.entries {
		err := mw.writeSomething(v)
		if err != nil {
			return cw.n, err
		}
	}
	err := mw.w.Flush()
	return cw.n, err
}

//NewBuilder creates a new Makefile Builder
func NewBuilder() *Builder {
	return new(Builder)
}
