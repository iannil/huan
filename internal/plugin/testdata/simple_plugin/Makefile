.PHONY: all clean

GO ?= go

all: simple_plugin.so

simple_plugin.so: main.go
	$(GO) build -buildmode=plugin -o $@ .

clean:
	rm -f *.so
