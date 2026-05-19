//go:build !windows

package rime

import (
	"context"
	"errors"
	"strings"

	"github.com/gaboolic/moqi-ime/input_methods/rime/cloudclipboard"
)

type cloudClipboardDialogResult struct {
	Enabled        bool
	BaseURL        string
	Username       string
	Password       string
	KeepPassword   bool
	SettingsRoot   string
	MinIntervalSec int
	ListHotkey     string
	TestOnly       bool
}

func promptCloudClipboardSettings(ctx context.Context, current cloudclipboard.Config, hasPassword bool) (cloudClipboardDialogResult, error) {
	return cloudClipboardDialogResult{}, errors.New("cloud clipboard settings dialog is only available on Windows")
}

func dialogResultToConfig(result cloudClipboardDialogResult, previous cloudclipboard.Config) cloudclipboard.Config {
	cfg := previous
	cfg.Enabled = result.Enabled
	cfg.BaseURL = cloudclipboard.NormalizeBaseURL(result.BaseURL)
	cfg.Username = strings.TrimSpace(result.Username)
	cfg.SettingsRoot = cloudclipboard.NormalizeDir(result.SettingsRoot)
	if result.MinIntervalSec > 0 {
		cfg.MinIntervalSec = result.MinIntervalSec
	}
	if hk := strings.TrimSpace(result.ListHotkey); hk != "" {
		cfg.ListHotkey = hk
	}
	if result.Password != "" {
		cfg.Password = result.Password
	} else if !result.KeepPassword {
		cfg.Password = ""
	}
	return cfg
}
