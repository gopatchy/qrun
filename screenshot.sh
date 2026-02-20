#!/bin/bash
exec go run ./cmd/qrunproxy/ -addr :0 --run-and-exit="shot-scraper http://localhost:{port}/ -o ${2:-/tmp/timeline.png} --width 1000 --height ${1:-1200}"
