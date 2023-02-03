.PHONY: all run vet test build cmd

all: vet build

run: build
	bin/loragw --cert-file=clientcert.crt --chain=chain.crt --key-file=private.key
vet:
	go vet ./...
	revive ./...
	golint ./...

test:
	go test -timeout 10s ./...

build: cmd

cmd:
	cd cmd/loragw && go build -o ../../bin/loragw

tools: 
	cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go get %

rpi:
	cd cmd/loragw && GOOS=linux GOARCH=arm go build -o ../../bin/loragw.rpi
