package main

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embeddedWeb embed.FS

func init() {
	sub, err := fs.Sub(embeddedWeb, "dist")
	if err == nil {
		WebFS = sub
	}
}

