// Package overlayfont embeds a font so the edge agent can burn a
// "camera - site" label into recorded video via ffmpeg's drawtext filter
// without depending on whatever fonts (if any) happen to be installed on
// the host — drawtext needs a real font file on disk, and font
// availability varies wildly across a fresh Raspberry Pi OS install, a
// minimal Ubuntu Server, and a dev machine. Bundling one removes that
// variable entirely.
//
// overlay-font.ttf is Roboto Bold, licensed under the Apache License 2.0
// (https://fonts.google.com/specimen/Roboto/about), fetched from Google
// Fonts.
package overlayfont

import (
	_ "embed"
	"os"
	"sync"
)

//go:embed overlay-font.ttf
var fontBytes []byte

var (
	once     sync.Once
	path     string
	writeErr error
)

// Path writes the embedded font out to a temp file (once per process,
// reused after that) and returns its path — ffmpeg's drawtext=fontfile=
// option needs a real filesystem path, it can't take font bytes directly.
func Path() (string, error) {
	once.Do(func() {
		f, err := os.CreateTemp("", "nvr-overlay-*.ttf")
		if err != nil {
			writeErr = err
			return
		}
		defer f.Close()
		if _, err := f.Write(fontBytes); err != nil {
			writeErr = err
			return
		}
		path = f.Name()
	})
	return path, writeErr
}
