//go:build !windows

package rime

func readCloudClipboardPassword() string  { return "" }
func saveCloudClipboardPassword(string) error { return nil }
