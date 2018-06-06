package makefile

import "strings"

//Text is an interface for a text value in a Makefile
type Text interface {
	Convert() string //return a string, panic to pass error
}

//RawText is a Text type which converts to the unmodified string
type RawText string

//Convert returns the unmodified string
func (raw RawText) Convert() string {
	return string(raw)
}

//ExtPattern is a simple Makefile pattern which matches an extension
//NOTE: no leading .
//TODO: error checking
//E.g. makefile.ExtPattern("go") -> %.go
type ExtPattern string

//Convert formats the ExtPattern as a Makefile pattern
func (ep ExtPattern) Convert() string {
	return "%." + string(ep)
}

//FilePath is a file path which can be used as Text
type FilePath string

//CheckValid does a validity check of a FilePath
//currently allows alphanumeric and "_ -.\/" runes
func (fp FilePath) CheckValid() error {
	if len(fp) == 0 {
		return ErrEmpty{"FilePath"}
	}
	for _, c := range fp {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		case c >= '0' && c <= '9':
		case c == '_':
		case c == ' ':
		case c == '-':
		case c == '.':
		case c == '\\':
		case c == '/':
		default:
			return ErrIllegalRune{
				Rune:      c,
				SrcString: string(fp),
				SrcType:   "FilePath",
			}
		}
	}
	return nil
}

//Convert converts a FilePath to a string suitable for a Makefile
//currently replaces ' ' -> '\ ' and '\' -> '\\'
func (fp FilePath) Convert() string {
	err := fp.CheckValid()
	if err != nil {
		panic(err)
	}
	return strings.Replace(strings.Replace(string(fp), " ", "\\ ", -1), "\\", "\\\\", -1)
}

//Join is a piece of text formed by joining other pieces of text
type Join struct {
	Sep string
	Txt []Text
}

//JoinText returns a Join formed by joining the txt values by the sep seperator
func JoinText(sep string, txt ...Text) Text {
	return Join{sep, txt}
}

//Convert returns the joined text
func (j Join) Convert() string {
	strs := make([]string, len(j.Txt))
	for i, v := range j.Txt {
		strs[i] = v.Convert()
	}
	return strings.Join(strs, j.Sep)
}
