package makefile

import (
	"fmt"
	"strings"
)

//MakeVar is a type which can be used as a Makefile variable
type MakeVar string

//Sub substitutes a MakeVar
func (mv MakeVar) Sub() MakeVarSubstitution {
	return MakeVarSubstitution{Variable: mv}
}

//CheckValid does a very strict validity check of a MakeVar
func (mv MakeVar) CheckValid() error {
	if len(mv) == 0 {
		return ErrEmpty{"MakeVar"}
	}
	for i, c := range mv {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		case c >= '0' && c <= '9':
			if i == 0 {
				return ErrIllegalRune{
					Rune:      c,
					SrcString: string(mv),
					SrcType:   "MakeVar",
					Context:   "initial",
				}
			}
		default:
			return ErrIllegalRune{
				Rune:      c,
				SrcString: string(mv),
				SrcType:   "MakeVar",
			}
		}
	}
	return nil
}

//MakeVarSubstitution is a Text type which substitutes for a Makefile variable
type MakeVarSubstitution struct {
	Variable MakeVar
}

//Convert returns the MakeVarSubstitution formatted as a Makefile variable substitution
func (v MakeVarSubstitution) Convert() string {
	err := v.Variable.CheckValid()
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("$(%s)", string(v.Variable))
}

type makeVarAssign struct {
	v   MakeVar
	val []Text
	op  string
}

func (mva makeVarAssign) String() string {
	vstrs := make([]string, len(mva.val))
	for i, v := range mva.val {
		vstrs[i] = v.Convert()
	}
	return fmt.Sprintf("%s %s %s", string(mva.v), mva.op, strings.Join(vstrs, " "))
}

//ShellVar is a type which can be used as a shell variable in a Makefile
type ShellVar string

//CheckValid does a validity check of a ShellVar
//based on http://pubs.opengroup.org/onlinepubs/000095399/basedefs/xbd_chap08.html
//allows lowercase because all modern systems should have lowercase
func (sv ShellVar) CheckValid() error {
	if len(sv) == 0 {
		return ErrEmpty{"ShellVar"}
	}
	for i, c := range sv {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'): //allow letters
		case c >= '0' && c <= '9': //handle numbers
			if i == 0 { //return an error if the variable starts with a number
				return ErrIllegalRune{
					Rune:      c,
					SrcString: string(sv),
					SrcType:   "ShellVar",
					Context:   "initial",
				}
			}
		case c == '_': //allow _
		default: //return error if there is a different rune
			return ErrIllegalRune{
				Rune:      c,
				SrcString: string(sv),
				SrcType:   "ShellVar",
			}
		}
	}
	return nil
}

//Sub substitutes a ShellVar
func (sv ShellVar) Sub() ShellVarSubstitution {
	return ShellVarSubstitution{Variable: sv}
}

//ShellVarSubstitution is a Text type which substitutes for a shell variable
type ShellVarSubstitution struct {
	Variable ShellVar
}

//Convert returns the ShellVarSubstitution formatted as a shell variable substitution
func (v ShellVarSubstitution) Convert() string {
	err := v.Variable.CheckValid()
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("$$%s", string(v.Variable))
}
