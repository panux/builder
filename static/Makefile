MATERIALIZE_VERSION = 1.0.0-beta
MEH_JS_VERSION = v0.1.4

all: materialize.min.js materialize.min.css meh.js meh.css

materialize.min.css:
	wget https://cdnjs.cloudflare.com/ajax/libs/materialize/$(MATERIALIZE_VERSION)/css/$@ -O $@
materialize.min.js:
	wget https://cdnjs.cloudflare.com/ajax/libs/materialize/$(MATERIALIZE_VERSION)/js/$@ -O $@
meh.js meh.css:
	wget https://github.com/jadr2ddude/meh.js/releases/download/$(MEH_JS_VERSION)/$@ -O $@
