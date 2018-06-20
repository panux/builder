package makefile

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

//makeWriter is a struct used to generate Makefiles from a builder
type makeWriter struct {
	w *bufio.Writer
}

//writeRaw writes a RawText value to the Makefile
func (mw makeWriter) writeRaw(raw RawText) error {
	_, err := mw.w.WriteString(string(raw))
	return err
}

//writeMakeVarAssign writes a makeVarAssign (variable assignment) to the Makefile
func (mw makeWriter) writeMakeVarAssign(mva makeVarAssign) (err error) {
	defer func() { //catch errors
		e := recover()
		if e != nil {
			err = e.(error)
		}
	}()
	return mw.writeRaw(RawText("\n" + mva.String()))
}

//writeComment writes a Comment to the Makefile
func (mw makeWriter) writeComment(c *Comment) error {
	c.afix()
	for _, l := range *c {
		_, err := fmt.Fprintf(mw.w, "\n# %s", l)
		if err != nil {
			return err
		}
	}
	return nil
}

//writeRule formats a rule and writes it to the Makefile
func (mw makeWriter) writeRule(r *Rule) (err error) {
	defer func() { //catch errors
		e := recover()
		if e != nil {
			err = e.(error)
		}
	}()

	//write .ONESHELL prefix if requested
	if r.Attr.OneShell {
		_, err = mw.w.WriteString("\n.ONESHELL:")
		if err != nil {
			return
		}
	}

	//format name of rule
	_, err = fmt.Fprintf(mw.w, "\n%s:", r.Name.Convert())
	if err != nil {
		return
	}

	//add shell specification if requested
	if r.Attr.Shell != nil {
		_, err = fmt.Fprintf(mw.w, " SHELL:=%s", r.Attr.Shell.Convert())
		if err != nil {
			return
		}
	}

	//add dependencies
	if len(r.Deps) != 0 {
		for _, v := range r.Deps {
			_, err = fmt.Fprintf(mw.w, " %s", v.Convert())
			if err != nil {
				return
			}
		}
	}

	//write out commands
	if len(r.Command) != 0 {
		for _, v := range r.Command {
			_, err = fmt.Fprintf(mw.w, "\n\t%s", v.String())
			if err != nil {
				return
			}
		}
	}

	return
}

//writeBuilder writes an entire builder to a makeWriter
func (mw makeWriter) writeBuilder(b *Builder) error {
	_, err := b.WriteTo(mw.w)
	return err
}

//writeSomething writes an entry to a Makefile
//calls type-appropriate method
func (mw makeWriter) writeSomething(i interface{}) error {
	switch v := i.(type) {
	case *Rule:
		return mw.writeRule(v)
	case RawText:
		return mw.writeRaw(v)
	case makeVarAssign:
		return mw.writeMakeVarAssign(v)
	case *Comment:
		return mw.writeComment(v)
	case *Builder:
		return mw.writeBuilder(v)
	default:
		return errors.New("Unsupported type write")
	}
}

//countWriter is a writer that counts the number of bytes it writes
type countWriter struct {
	w io.Writer
	n int64
}

func (cw *countWriter) Write(dat []byte) (int, error) {
	n, err := cw.w.Write(dat)
	cw.n += int64(n)
	return n, err
}
