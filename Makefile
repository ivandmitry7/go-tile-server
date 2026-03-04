APP_NAME := tile-server
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: build run clean vet test cross docker

build:
	go build -ldflags="$(LDFLAGS)" -o $(APP_NAME) .

run: build
	./$(APP_NAME)

vet:
	go vet ./...

test:
	go test ./...

clean:
	rm -f $(APP_NAME) tile-server-*

cross:
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(APP_NAME)-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(APP_NAME)-linux-arm64 .
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(APP_NAME)-darwin-amd64 .
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(APP_NAME)-darwin-arm64 .
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(APP_NAME)-windows-amd64.exe .

docker:
	docker build -t $(APP_NAME):$(VERSION) .
