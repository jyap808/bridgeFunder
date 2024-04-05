#!/bin/bash

VERSION=1.0
APP='ubqbridgefunder'

docker build -t julian/${APP}:${VERSION} .
