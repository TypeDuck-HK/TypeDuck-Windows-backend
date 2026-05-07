//go:build !android

package rime

func androidRimeDirs() (sharedDir string, userDir string, ok bool) {
	return "", "", false
}
