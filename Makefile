.PHONY: all priv run vet test build cmd

ifeq ($(GOPRIVATE),)
GOPRIVATE := github.com/lab5e/l5log
endif
all: vet priv build

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

cmd:
	cd cmd/loragw && go build -o ../../bin/loragw

tools: 
	cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go get %

