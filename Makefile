.PHONY: build test vet

build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o bin/vnode ./cmd/vnode

test:
	go test ./...

vet:
	go vet ./...
