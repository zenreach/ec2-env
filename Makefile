name=ec2-env
export GOBIN=$(shell pwd)
export GOPATH=$(shell pwd)/.go

.PHONY: all build clean test

all: build

build:
	mkdir -p $(GOPATH)
	go get -d .
	go build
	strip ${name}

clean:
	rm -rf $(GOPATH) $(name)

test:
	test -z "$(shell gofmt -s -l main.go)"
	go vet main.go
