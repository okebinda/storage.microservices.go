.PHONY: build clean deploy gomodgen

build: gomodgen
	export GO111MODULE=on
	env GOOS=linux go build -ldflags="-s -w" -o bin/upload-url upload-url/main.go
	env GOOS=linux go build -ldflags="-s -w" -o bin/upload-image upload-image/main.go
	env GOOS=linux go build -ldflags="-s -w" -o bin/upload-image-callback upload-image-callback/main.go

clean:
	rm -rf ./bin ./vendor Gopkg.lock

deploy: clean build
	sls deploy --verbose

gomodgen:
	chmod u+x gomod.sh
	./gomod.sh
