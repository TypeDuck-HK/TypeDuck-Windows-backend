package cloudclipboard

import "strings"

type Config struct {
	Enabled         bool
	BaseURL         string
	Username        string
	Password        string
	SettingsRoot    string
	MinIntervalSec  int
	ListHotkey      string
}

func (c Config) MinIntervalMs() int64 {
	sec := c.MinIntervalSec
	if sec < 1 {
		sec = 10
	}
	return int64(sec) * 1000
}

func (c Config) IsComplete() bool {
	return c.BaseURL != "" && c.Username != "" && c.Password != ""
}

func (c Config) SettingsRootURL() string {
	base := strings.TrimRight(c.BaseURL, "/")
	path := strings.Trim(strings.TrimPrefix(c.SettingsRoot, "/"), "/")
	return base + "/" + path + "/"
}

func (c Config) ClipDirectoryURL() string {
	base := strings.TrimRight(c.BaseURL, "/")
	clipPath := strings.TrimPrefix(ClipPathUnder(c.SettingsRoot), "/")
	return base + "/" + clipPath
}

func NormalizeBaseURL(raw string) string {
	return strings.TrimSpace(raw)
}

func IsAllowedBaseURL(url string) bool {
	u := strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func RejectionMessage() string {
	return "WebDAV 地址须以 http:// 或 https:// 开头"
}
