.PHONY: build build-web build-go dev test clean

# build: vite bundle → go:embed → single binary ./swormery
build: build-web build-go

build-web:
	cd web && npm install && npm run build

build-go:
	go build -o swormery ./cmd/swormery

# dev: run the API daemon and the vite dev server (proxying /api to :7777)
dev:
	go run ./cmd/swormery serve & \
	cd web && npm run dev; \
	kill %1 2>/dev/null

test:
	go vet ./...
	go test ./...

clean:
	rm -f swormery
	find web/dist -mindepth 1 ! -name .gitkeep -delete
