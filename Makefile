.PHONY: build build-arm64 test vet

build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o bin/vnode ./cmd/vnode

build-arm64:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o bin/vnode-arm64 ./cmd/vnode

test:
	go test ./...

vet:
	go vet ./...
