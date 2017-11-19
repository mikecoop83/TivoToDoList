#!/bin/bash

go build -o ./bin/TivoToDoList src/*.go
cp src/TivoToDoList.conf bin/

