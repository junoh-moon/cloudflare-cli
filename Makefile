PREFIX=$$HOME/.local
APP_NAME=cloudflare
BIN_DIR=$(PREFIX)/bin

.PHONY: build test install uninstall clean

# A static binary: it needs nothing else on the machine it runs on.
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(APP_NAME) .

test:
	go vet ./...
	go test ./...

# The binary is copied next to its destination and then renamed over it, so
# a cron job starting mid-install sees either the old or the new one.
install: build
	mkdir -p $(BIN_DIR)
	install -m 755 $(APP_NAME) $(BIN_DIR)/.$(APP_NAME).new
	mv -f $(BIN_DIR)/.$(APP_NAME).new $(BIN_DIR)/$(APP_NAME)

uninstall:
	rm -f $(BIN_DIR)/$(APP_NAME)

clean:
	rm -f $(APP_NAME)
