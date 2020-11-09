#!/bin/sh

HIGHLIGHT_COLOR="\e[1;36m" # cyan
DEFAULT_COLOR="\e[0m"

cd /vagrant/services/image-serve

echo "\n${HIGHLIGHT_COLOR}Running gofmt...${DEFAULT_COLOR}"
gofmt -l -s -w .

echo "\n${HIGHLIGHT_COLOR}Running go vet...${DEFAULT_COLOR}"
export CGO_ENABLED='0'; go vet ./...

echo "\n${HIGHLIGHT_COLOR}Running golint...${DEFAULT_COLOR}"
golint ./...

echo "\n${HIGHLIGHT_COLOR}Running gosec...${DEFAULT_COLOR}"
gosec ./...
