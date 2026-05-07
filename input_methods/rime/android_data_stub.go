//go:build !android

package rime

func SetAndroidDataDir(path string) {}

func androidAppDataRoot() string {
	return ""
}

func androidRimeDirs() (sharedDir string, userDir string, ok bool) {
	return "", "", false
}
