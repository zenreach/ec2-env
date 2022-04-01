name=ec2-env
export GOBIN=$(shell pwd)
export GOPATH=$(shell pwd)/.go

.PHONY: all build clean test

all: build

build:
	go build
	strip ${name}

clean:
	rm -rf $(GOPATH) $(name)

test:
	gofmt -s -l main.go
	go vet main.go
