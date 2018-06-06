package makefile

import (
	"fmt"
)

//ErrIllegalRune is an error type returned indicating that a rune is not allowed
type ErrIllegalRune struct {
	Rune      rune   //the illegal rune
	SrcString string //the string it was found in
	SrcType   string //the type of the source string (MakeVar, etc.)
	Context   string //the subtype of error (e.g. "initial")
}

func (err ErrIllegalRune) Error() string {
	cont := err.Context
	if cont != "" {
		cont = fmt.Sprintf(" %s", cont)
	}
	return fmt.Sprintf("makefile: illegal%s rune %q in %q (a %s)", cont, err.Rune, err.SrcString, err.SrcType)
}

//ErrEmpty is an error returned when something is empty
type ErrEmpty struct {
	SrcType string
}

func (err ErrEmpty) Error() string {
	return fmt.Sprintf("makefile: empty %s", err.SrcType)
}
