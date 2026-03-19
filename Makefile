.PHONY: run build test clean docker

run:
	go run ./cmd/server

build:
	@mkdir -p bin
	go build -o bin/server ./cmd/server

test:
	go test ./...

clean:
	rm -rf bin/

docker:
	docker build -t zonemeister .
