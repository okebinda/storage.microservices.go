.PHONY: build clean deploy

build:
	env GOOS=linux go build -ldflags="-s -w" -o bin/image-resize-ratio image-resize-ratio/main.go
	env GOOS=linux go build -ldflags="-s -w" -o bin/image-resize-crop image-resize-crop/main.go

clean:
	rm -rf ./bin

deploy: clean build
	sls deploy --verbose
