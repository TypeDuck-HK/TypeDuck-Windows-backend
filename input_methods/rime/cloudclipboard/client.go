package cloudclipboard

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const propfindBody = `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:getlastmodified/>
    <d:displayname/>
  </d:prop>
</d:propfind>`

type ClipEntry struct {
	Name         string
	LastModified int64
}

type Client struct {
	cfg    Config
	client *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) TestConnection() error {
	return c.ensureRemoteDirectory()
}

func (c *Client) ListClips() ([]ClipEntry, error) {
	if err := c.ensureRemoteDirectory(); err != nil {
		return nil, err
	}
	resp, err := c.propfind(c.cfg.ClipDirectoryURL(), 1)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("PROPFIND failed: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	entries := parsePropfind(string(body))
	var clips []ClipEntry
	seen := map[string]bool{}
	for _, e := range entries {
		if !IsClipFileName(e.Name) || seen[e.Name] {
			continue
		}
		seen[e.Name] = true
		clips = append(clips, e)
	}
	// sort descending by last modified
	for i := 0; i < len(clips); i++ {
		for j := i + 1; j < len(clips); j++ {
			if clips[j].LastModified > clips[i].LastModified {
				clips[i], clips[j] = clips[j], clips[i]
			}
		}
	}
	return clips, nil
}

func (c *Client) UploadClip(filename, text string) error {
	if err := c.ensureRemoteDirectory(); err != nil {
		return err
	}
	fileURL := c.fileURL(filename)
	req, err := http.NewRequest(http.MethodPut, fileURL, strings.NewReader(text))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("PUT failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) DownloadClip(name string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.fileURL(name), nil)
	if err != nil {
		return "", err
	}
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("GET failed: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) DeleteClip(name string) error {
	if !IsClipFileName(name) {
		return fmt.Errorf("invalid clip filename")
	}
	req, err := http.NewRequest(http.MethodDelete, c.fileURL(name), nil)
	if err != nil {
		return err
	}
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 && resp.StatusCode != 404 {
		return fmt.Errorf("DELETE failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) ensureRemoteDirectory() error {
	rootURL := strings.TrimRight(c.cfg.BaseURL, "/") + "/"
	resp, err := c.propfind(rootURL, 0)
	if err != nil {
		return err
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case 401:
		return fmt.Errorf("认证失败：请检查用户名和密码。飞牛须使用「可见文件夹范围」所属账号登录")
	case 200, 201, 204, 207:
	default:
		return fmt.Errorf("无法访问 WebDAV 根路径 HTTP %d。飞牛地址请填 http://192.168.x.x:5005/（注意末尾 /）", resp.StatusCode)
	}

	settingsRoot := c.cfg.SettingsRootURL()
	if !c.directoryExists(settingsRoot) {
		if err := c.mkcol(settingsRoot); err != nil {
			return fmt.Errorf("设置目录 %s 不存在且无法自动创建：%w", c.cfg.SettingsRoot, err)
		}
		if !c.directoryExists(settingsRoot) {
			return fmt.Errorf("设置目录 %s 不存在且无法自动创建。请在 NAS 文件管理中手动新建，或把设置目录改为 /", c.cfg.SettingsRoot)
		}
	}

	clipURL := strings.TrimRight(c.cfg.ClipDirectoryURL(), "/") + "/"
	if c.directoryExists(clipURL) {
		return nil
	}
	if err := c.mkcol(clipURL); err == nil && c.directoryExists(clipURL) {
		return nil
	}
	return fmt.Errorf("剪贴板目录 %s 无法创建。请在 %s 下手动新建 clip 文件夹", ClipDir, c.cfg.SettingsRoot)
}

func (c *Client) directoryExists(dirURL string) bool {
	resp, err := c.propfind(dirURL, 0)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode <= 299
}

func (c *Client) propfind(targetURL string, depth int) (*http.Response, error) {
	req, err := http.NewRequest("PROPFIND", targetURL, bytes.NewBufferString(propfindBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", fmt.Sprintf("%d", depth))
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	c.setAuth(req)
	return c.client.Do(req)
}

func (c *Client) mkcol(targetURL string) error {
	req, err := http.NewRequest("MKCOL", targetURL, nil)
	if err != nil {
		return err
	}
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 && resp.StatusCode != 405 {
		return fmt.Errorf("MKCOL failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) fileURL(filename string) string {
	dir := strings.TrimRight(c.cfg.ClipDirectoryURL(), "/")
	safeName := strings.TrimPrefix(filename, "/")
	return dir + "/" + safeName
}

func (c *Client) setAuth(req *http.Request) {
	req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
}

func parsePropfind(xmlText string) []ClipEntry {
	if strings.TrimSpace(xmlText) == "" {
		return nil
	}
	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	var results []ClipEntry
	var inResponse bool
	var href string
	var lastModified int64
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(localName(t.Name))
			switch name {
			case "response":
				inResponse = true
				href = ""
				lastModified = 0
			case "href":
				if inResponse {
					var text string
					_ = decoder.DecodeElement(&text, &t)
					href = strings.TrimSpace(text)
				}
			case "getlastmodified":
				if inResponse {
					var text string
					_ = decoder.DecodeElement(&text, &t)
					lastModified = parseHTTPDate(strings.TrimSpace(text))
				}
			}
		case xml.EndElement:
			if strings.ToLower(localName(t.Name)) == "response" && inResponse {
				if name := nameFromHref(href); name != "" && name != "." && name != ".." {
					results = append(results, ClipEntry{Name: name, LastModified: lastModified})
				}
				inResponse = false
			}
		}
	}
	return results
}

func localName(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	parts := strings.Split(name.Local, ":")
	return parts[len(parts)-1]
}

func nameFromHref(href string) string {
	decoded, err := url.PathUnescape(strings.TrimSpace(href))
	if err != nil {
		decoded = strings.TrimSpace(href)
	}
	if decoded == "" {
		return ""
	}
	decoded = strings.TrimRight(decoded, "/")
	if i := strings.LastIndex(decoded, "/"); i >= 0 {
		return decoded[i+1:]
	}
	return decoded
}

func parseHTTPDate(raw string) int64 {
	layouts := []string{
		time.RFC1123,
		"Mon, 02 Jan 2006 15:04:05 GMT",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}
