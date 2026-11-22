# Needs Go >= 1.23. The binary is static: build it on the Mac/PC and copy it to the box.
BIN := bin
PKG := ./cmd/tvcast
BOX ?= tvbox

.PHONY: build arm64 test run deploy clean
build:
	go build -o $(BIN)/tvcast $(PKG)

arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN)/tvcast-linux-arm64 $(PKG)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN)/tvcast-remote-linux-arm64 ./cmd/tvcast-remote

test:
	go vet ./...
	go test ./...

# Local test with the web viewer: open http://localhost:8080/tv in one tab
# and http://localhost:8080 in another (or from the phone with the URL in the log).
run: build
	$(BIN)/tvcast -data ./data -display web

# Copy the binary to the box (restarts nothing: do it yourself with systemctl).
deploy: arm64
	scp $(BIN)/tvcast-linux-arm64 $(BOX):/tmp/tvcast
	@echo "On the box: sudo install -m755 /tmp/tvcast /usr/local/bin/tvcast"

clean:
	rm -rf $(BIN) data
