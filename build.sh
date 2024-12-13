#!/bin/bash

export DDNS_GO_VERSION=v6.7.7
docker build --platform linux/amd64 -t yilee01/ddns-go .
