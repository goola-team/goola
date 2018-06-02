# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: goola android ios goola-cross swarm evm all test clean
.PHONY: goola-linux goola-linux-386 goola-linux-amd64 goola-linux-mips64 goola-linux-mips64le
.PHONY: goola-linux-arm goola-linux-arm-5 goola-linux-arm-6 goola-linux-arm-7 goola-linux-arm64
.PHONY: goola-darwin goola-darwin-386 goola-darwin-amd64
.PHONY: goola-windows goola-windows-386 goola-windows-amd64

GOBIN = $(shell pwd)/build/bin
GO ?= latest

goola:
	build/env.sh go run build/ci.go install ./cmd/goola
	@echo "Done building."
	@echo "Run \"$(GOBIN)/goola\" to launch goola."

swarm:
	build/env.sh go run build/ci.go install ./cmd/swarm
	@echo "Done building."
	@echo "Run \"$(GOBIN)/swarm\" to launch swarm."

all:
	build/env.sh go run build/ci.go install

android:
	build/env.sh go run build/ci.go aar --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/goola.aar\" to use the library."

ios:
	build/env.sh go run build/ci.go xcode --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/goola.framework\" to use the library."

test: all
	build/env.sh go run build/ci.go test

clean:
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

devtools:
	env GOBIN= go get -u golang.org/x/tools/cmd/stringer
	env GOBIN= go get -u github.com/kevinburke/go-bindata/go-bindata
	env GOBIN= go get -u github.com/fjl/gencodec
	env GOBIN= go get -u github.com/golang/protobuf/protoc-gen-go
	env GOBIN= go install ./cmd/abigen
	@type "npm" 2> /dev/null || echo 'Please install node.js and npm'
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'

# Cross Compilation Targets (xgo)

goola-cross: goola-linux goola-darwin goola-windows goola-android goola-ios
	@echo "Full cross compilation done:"
	@ls -ld $(GOBIN)/goola-*

goola-linux: goola-linux-386 goola-linux-amd64 goola-linux-arm goola-linux-mips64 goola-linux-mips64le
	@echo "Linux cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-*

goola-linux-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/386 -v ./cmd/goola
	@echo "Linux 386 cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep 386

goola-linux-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/amd64 -v ./cmd/goola
	@echo "Linux amd64 cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep amd64

goola-linux-arm: goola-linux-arm-5 goola-linux-arm-6 goola-linux-arm-7 goola-linux-arm64
	@echo "Linux ARM cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep arm

goola-linux-arm-5:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-5 -v ./cmd/goola
	@echo "Linux ARMv5 cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep arm-5

goola-linux-arm-6:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-6 -v ./cmd/goola
	@echo "Linux ARMv6 cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep arm-6

goola-linux-arm-7:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-7 -v ./cmd/goola
	@echo "Linux ARMv7 cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep arm-7

goola-linux-arm64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm64 -v ./cmd/goola
	@echo "Linux ARM64 cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep arm64

goola-linux-mips:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips --ldflags '-extldflags "-static"' -v ./cmd/goola
	@echo "Linux MIPS cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep mips

goola-linux-mipsle:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mipsle --ldflags '-extldflags "-static"' -v ./cmd/goola
	@echo "Linux MIPSle cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep mipsle

goola-linux-mips64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64 --ldflags '-extldflags "-static"' -v ./cmd/goola
	@echo "Linux MIPS64 cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep mips64

goola-linux-mips64le:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64le --ldflags '-extldflags "-static"' -v ./cmd/goola
	@echo "Linux MIPS64le cross compilation done:"
	@ls -ld $(GOBIN)/goola-linux-* | grep mips64le

goola-darwin: goola-darwin-386 goola-darwin-amd64
	@echo "Darwin cross compilation done:"
	@ls -ld $(GOBIN)/goola-darwin-*

goola-darwin-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/386 -v ./cmd/goola
	@echo "Darwin 386 cross compilation done:"
	@ls -ld $(GOBIN)/goola-darwin-* | grep 386

goola-darwin-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/amd64 -v ./cmd/goola
	@echo "Darwin amd64 cross compilation done:"
	@ls -ld $(GOBIN)/goola-darwin-* | grep amd64

goola-windows: goola-windows-386 goola-windows-amd64
	@echo "Windows cross compilation done:"
	@ls -ld $(GOBIN)/goola-windows-*

goola-windows-386:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/386 -v ./cmd/goola
	@echo "Windows 386 cross compilation done:"
	@ls -ld $(GOBIN)/goola-windows-* | grep 386

goola-windows-amd64:
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/amd64 -v ./cmd/goola
	@echo "Windows amd64 cross compilation done:"
	@ls -ld $(GOBIN)/goola-windows-* | grep amd64
