package makefile

import "strings"

//Comment is a comment in a Makefile
type Comment []string

//Line adds a line to the comment
func (c *Comment) Line(ln string) *Comment {
	if strings.Contains(ln, "\n") {
		for _, l := range strings.Split(ln, "\n") {
			c.Line(l)
		}
		return c
	}
	*c = append(*c, ln)
	return c
}

func (c *Comment) afix() {
	if c.needfix() {
		c.fix()
	}
}

func (c *Comment) needfix() bool {
	for _, v := range *c {
		if strings.Contains(v, "\n") {
			return true
		}
	}
	return false
}

func (c *Comment) fix() {
	*c = Comment(strings.Split(strings.Join([]string(*c), "\n"), "\n"))
}
