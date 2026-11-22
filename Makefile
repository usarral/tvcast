# Requiere Go >= 1.23. El binario es estático: se compila en el Mac/PC y se copia a la caja.
BIN := bin
PKG := ./cmd/tvcast
BOX ?= tvbox

.PHONY: build arm64 test run deploy clean
build:
	go build -o $(BIN)/tvcast $(PKG)

arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN)/tvcast-linux-arm64 $(PKG)

test:
	go vet ./...
	go test ./...

# Prueba en local con visor web: abre http://localhost:8080/tv en una pestaña
# y http://localhost:8080 en otra (o desde el móvil con la URL del log).
run: build
	$(BIN)/tvcast -data ./data -display web

# Copia el binario a la caja (no reinicia nada: hazlo tú con systemctl).
deploy: arm64
	scp $(BIN)/tvcast-linux-arm64 $(BOX):/tmp/tvcast
	@echo "En la caja: sudo install -m755 /tmp/tvcast /usr/local/bin/tvcast"

clean:
	rm -rf $(BIN) data
