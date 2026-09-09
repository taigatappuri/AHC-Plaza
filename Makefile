APP := ahc-plaza
VERSION ?= 0.1.0-dev
INSTALL_DIR ?= $(HOME)/.local/bin
GO ?= go
NPM ?= npm
PYTHON ?= python3
ARCH ?= $(shell $(GO) env GOARCH)

.PHONY: web-install web-build web-check test vet check build install-local uninstall-local release clean tuning-runtime

web-install:
	cd web && $(NPM) ci

web-build:
	cd web && $(NPM) run build

web-check:
	cd web && $(NPM) run check

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

check: test vet web-check

tuning-runtime:
	uv run --with zstandard==0.25.0 --python $(PYTHON) scripts/tuning/build_runtime.py --arch $(ARCH)

build: web-build tuning-runtime
	CGO_ENABLED=0 $(GO) build -tags tuning_bundle -trimpath -ldflags "-X main.version=$(VERSION)" -o $(APP) ./cmd/ahc-plaza

install-local: build
	install -d "$(INSTALL_DIR)"
	install -m 0755 $(APP) "$(INSTALL_DIR)/$(APP)"

uninstall-local:
	rm -f "$(INSTALL_DIR)/$(APP)"

release: web-build
	uv run --with zstandard==0.25.0 --python $(PYTHON) scripts/tuning/build_runtime.py --arch amd64
	uv run --with zstandard==0.25.0 --python $(PYTHON) scripts/tuning/build_runtime.py --arch arm64
	install -d dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -tags tuning_bundle -trimpath -ldflags "-X main.version=$(VERSION)" -o dist/ahc-plaza-linux-amd64 ./cmd/ahc-plaza
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -tags tuning_bundle -trimpath -ldflags "-X main.version=$(VERSION)" -o dist/ahc-plaza-linux-arm64 ./cmd/ahc-plaza
	cd dist && sha256sum ahc-plaza-linux-amd64 ahc-plaza-linux-arm64 > checksums.txt

clean:
	go clean
