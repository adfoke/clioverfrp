BIN_DIR := bin

.PHONY: build clean bundle install-lagent

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/lagent ./cmd/lagent
	go build -o $(BIN_DIR)/ragent ./cmd/ragent

bundle: build
	go run ./cmd/bundle

install-lagent:
	sh ./scripts/install-lagent.sh

clean:
	rm -rf $(BIN_DIR)
