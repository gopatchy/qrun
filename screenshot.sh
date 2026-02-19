#!/bin/bash
exec go run ./cmd/qrunweb/ "--run-and-exit=shot-scraper http://localhost:8080/ -o /tmp/timeline.png --width 1000 --height ${1:-1200}"
