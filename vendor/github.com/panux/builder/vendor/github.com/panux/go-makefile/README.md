# go-makefile [![GoDoc](https://godoc.org/github.com/panux/go-makefile?status.svg)](https://godoc.org/github.com/panux/go-makefile) [![Build Status](https://travis-ci.org/panux/go-makefile.svg?branch=master)](https://travis-ci.org/panux/go-makefile)
A go package for generating Makefiles

## Example
See [example/self.go](https://github.com/panux/go-makefile/blob/master/example/self.go) for an example. To try this first run ````go run self.go````. This will generate self.mk. Now you can run ````make```` and the generated makefile will compile self.go and use it to re-generate the makefile.
