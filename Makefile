.PHONY: all run vet test build cmd priv tools rpi dockerhub

GOPRIVATE := github.com/lab5e/lospan,github.com/lab5e/l5log

VERSION := $(shell reto version)

all: vet build

priv:
	go env -w GOPRIVATE=$(GOPRIVATE)

run: build 
	bin/loragw --cert-file=clientcert.crt --chain=chain.crt --key-file=private.key

vet:
	go vet ./...
	revive ./...
	golint ./...

test:
	go test -timeout 10s ./...

build: cmd 

cmd: priv
	cd cmd/loragw && go build -o ../../bin/loragw

tools: 
	cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go get %

arch: priv
	cd cmd/loragw && GOOS=linux GOARCH=arm go build -o ../../bin/loragw.linux-arm
	cd cmd/loragw && GOOS=linux GOARCH=amd64 go build -o ../../bin/loragw.linux-amd64
	cd cmd/loragw && GOOS=linux GOARCH=arm64 go build -o ../../bin/loragw.linux-arm64
	cd cmd/loragw && GOOS=windows GOARCH=amd64 go build -o ../../bin/loragw.exe
	cd cmd/loragw && GOOS=darwin GOARCH=arm64 go build -o ../../bin/loragw.macos-arm64
	cd cmd/loragw && GOOS=darwin GOARCH=amd64 go build -o ../../bin/loragw.macos-amd64
	
dockerhub: arch
# This requires a docker login up front (docker login --username=lab5e --email=stalehd@lab5e.com)
	docker buildx build \
		--platform linux/arm/v7,linux/arm64,linux/amd64 . \
		--tag lab5e/loragw:latest \
		--tag lab5e/loragw:v${VERSION} \
		--push

rpi:
	cd cmd/loragw && GOOS=linux GOARCH=arm go build -o ../../bin/loragw.rpi
