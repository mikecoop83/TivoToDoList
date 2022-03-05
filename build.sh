#!/bin/bash

echo "Building arm..."
rm -rf ./bin/arm/*
env GOOS=linux GOARCH=arm go1.18rc1 build -o ./bin/arm/TivoToDoList *.go
echo "Building amd64..."
rm -rf ./bin/amd64/*
env GOOS=linux GOARCH=amd64 go1.18rc1 build -o ./bin/amd64/TivoToDoList *.go

