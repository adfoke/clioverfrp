BIN_DIR := bin
DIST_DIR := dist
HOST_GOOS := $(shell go env GOOS)
HOST_GOARCH := $(shell go env GOARCH)

LINUX_GOOS := linux
LINUX_GOARCH := amd64
DARWIN_GOOS := darwin
DARWIN_GOARCH := arm64
WINDOWS_GOOS := windows
WINDOWS_GOARCH := amd64

.PHONY: build clean bundle install-lagent

build:
	mkdir -p $(BIN_DIR)
	mkdir -p $(DIST_DIR)/$(LINUX_GOOS)-$(LINUX_GOARCH)
	mkdir -p $(DIST_DIR)/$(DARWIN_GOOS)-$(DARWIN_GOARCH)
	mkdir -p $(DIST_DIR)/$(WINDOWS_GOOS)-$(WINDOWS_GOARCH)
	go build -o $(BIN_DIR)/lagent ./cmd/lagent
	go build -o $(BIN_DIR)/ragent ./cmd/ragent
	GOOS=$(LINUX_GOOS) GOARCH=$(LINUX_GOARCH) go build -o $(DIST_DIR)/$(LINUX_GOOS)-$(LINUX_GOARCH)/lagent ./cmd/lagent
	GOOS=$(LINUX_GOOS) GOARCH=$(LINUX_GOARCH) go build -o $(DIST_DIR)/$(LINUX_GOOS)-$(LINUX_GOARCH)/ragent ./cmd/ragent
	GOOS=$(DARWIN_GOOS) GOARCH=$(DARWIN_GOARCH) go build -o $(DIST_DIR)/$(DARWIN_GOOS)-$(DARWIN_GOARCH)/lagent ./cmd/lagent
	GOOS=$(DARWIN_GOOS) GOARCH=$(DARWIN_GOARCH) go build -o $(DIST_DIR)/$(DARWIN_GOOS)-$(DARWIN_GOARCH)/ragent ./cmd/ragent
	GOOS=$(WINDOWS_GOOS) GOARCH=$(WINDOWS_GOARCH) go build -o $(DIST_DIR)/$(WINDOWS_GOOS)-$(WINDOWS_GOARCH)/lagent.exe ./cmd/lagent
	GOOS=$(WINDOWS_GOOS) GOARCH=$(WINDOWS_GOARCH) go build -o $(DIST_DIR)/$(WINDOWS_GOOS)-$(WINDOWS_GOARCH)/ragent.exe ./cmd/ragent

bundle: build
	go run ./cmd/bundle

install-lagent:
	sh ./scripts/install-lagent.sh

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
