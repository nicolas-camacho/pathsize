BINARY  := pathsize
PKG     := github.com/nicolas-camacho/pathsize
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST    := dist

# Platforms to cross-compile for: GOOS/GOARCH
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

.PHONY: all build run fmt vet tidy clean dist $(PLATFORMS)

all: fmt vet build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

run:
	go run . $(ARGS)

fmt:
	gofmt -w main.go

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf $(DIST) $(BINARY) $(BINARY).exe

# Cross-compile every platform into dist/
dist: $(PLATFORMS)

$(PLATFORMS):
	$(eval GOOS := $(word 1,$(subst /, ,$@)))
	$(eval GOARCH := $(word 2,$(subst /, ,$@)))
	$(eval EXT := $(if $(filter windows,$(GOOS)),.exe,))
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" \
		-o $(DIST)/$(BINARY)_$(GOOS)_$(GOARCH)$(EXT) .
