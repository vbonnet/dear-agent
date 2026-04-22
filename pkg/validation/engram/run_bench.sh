#!/bin/bash
cd "$(dirname "$0")"
go test -bench=. -benchmem
