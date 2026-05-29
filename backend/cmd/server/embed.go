package main

import (
	"embed"
	"io/fs"
)

//go:embed all:web/*
var webFS embed.FS

// staticFS returns the embedded frontend as a sub-FS rooted at `web`.
// During development, if the dir is empty, the server simply serves API only.
func staticFS() (fs.FS, error) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, err
	}
	// detect whether anything actually exists
	entries, err := fs.ReadDir(sub, ".")
	if err != nil || len(entries) == 0 {
		return nil, nil
	}
	// has only the placeholder? treat as empty
	if len(entries) == 1 && entries[0].Name() == ".gitkeep" {
		return nil, nil
	}
	return sub, nil
}
