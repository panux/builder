package makefile

//Target is the target of the current rule
var Target = RawText("$@")

//Dep1 is the first dep of the current rule
var Dep1 = RawText("$<")
