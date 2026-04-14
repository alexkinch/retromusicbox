.PHONY: all build build-frontend build-backend clean dev

all: build

build: build-frontend build-backend

build-frontend:
	cd web/channel && npm install && npm run build

build-backend:
	go build -o rmbd ./cmd/rmbd
	go build -o rmbctl ./cmd/rmbctl

clean:
	rm -f rmbd rmbctl
	rm -rf web/channel/dist cmd/rmbd/static

dev-frontend:
	cd web/channel && npm run dev

dev-backend:
	go run ./cmd/rmbd --config configs/config.yaml

init-db:
	go run ./cmd/rmbctl init-db
