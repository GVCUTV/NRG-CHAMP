#!/bin/bash

docker build -t ledger-service .
docker run -d -p 8087:8080 --name ledger-service ledger-service
docker ps -a