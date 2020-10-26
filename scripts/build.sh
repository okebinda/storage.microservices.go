#!/bin/bash

HIGHLIGHT_COLOR=$'\e[1;36m' # cyan
DEFAULT_COLOR=$'\e[0m'

echo -e "\n${HIGHLIGHT_COLOR}Installing tools...${DEFAULT_COLOR}"

# go tools
go get -u golang.org/x/lint/golint
go get github.com/securego/gosec/cmd/gosec
go get github.com/githubnemo/CompileDaemon


echo -e "\n${HIGHLIGHT_COLOR}Build complete.${DEFAULT_COLOR}\n"
