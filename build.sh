#!/bin/bash

echo "Building arm64..."
GOARCH=arm64 go build -o ./bin/arm64/TivoToDoList src/*.go
echo "Building amd64..."
GOARCH=amd64 go build -o ./bin/amd64/TivoToDoList src/*.go

cp src/TivoToDoList.conf bin/

