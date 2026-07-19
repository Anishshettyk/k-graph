// Package main is the entrypoint for the kgraph CLI.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/anishetty/kgraph/cmd"
)

// uiFiles holds the compiled React frontend. Build it first:
//
//	cd ui && npm install && npm run build
//
//go:embed all:ui/dist
var uiFiles embed.FS

func main() {
	cmd.SetUIFiles(uiFiles)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
