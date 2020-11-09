#!/bin/sh

HIGHLIGHT_COLOR="\e[1;36m" # cyan
DEFAULT_COLOR="\e[0m"

cd /vagrant/services/image-serve

echo "\n${HIGHLIGHT_COLOR}Installing dependencies...${DEFAULT_COLOR}"
go get ./...

echo "\n${HIGHLIGHT_COLOR}Build complete.${DEFAULT_COLOR}\n"
