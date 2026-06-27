package rime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gaboolic/moqi-ime/imecore"
)

const (
	maxSchemeSetArchiveBytes   = 200 << 20
	maxSchemeSetExtractedBytes = 500 << 20
)

type schemeSetDownloadPackage struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
}

type schemeSetHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type schemeSetDownloadState struct {
	mu      sync.Mutex
	running bool
}

var (
	sharedSchemeSetDownloadState schemeSetDownloadState
	schemeSetDownloadHTTPClient  schemeSetHTTPDoer = &http.Client{Timeout: 2 * time.Minute}
	schemeSetDownloadPromptFunc                    = promptSchemeSetDownloadPackage
)

func resetSchemeSetDownloadStateForTest() {
	sharedSchemeSetDownloadState.mu.Lock()
	sharedSchemeSetDownloadState.running = false
	sharedSchemeSetDownloadState.mu.Unlock()
	schemeSetDownloadHTTPClient = &http.Client{Timeout: 2 * time.Minute}
	schemeSetDownloadPromptFunc = promptSchemeSetDownloadPackage
}

func (ime *IME) downloadSchemeSetAsync(req *imecore.Request, resp *imecore.Response) bool {
	sharedSchemeSetDownloadState.mu.Lock()
	if sharedSchemeSetDownloadState.running {
		sharedSchemeSetDownloadState.mu.Unlock()
		if resp != nil {
			resp.TrayNotification = trayNotification("Scheme-set download is already running", imecore.TrayNotificationIconInfo)
		}
		return false
	}
	sharedSchemeSetDownloadState.running = true
	sharedSchemeSetDownloadState.mu.Unlock()

	if resp != nil {
		resp.TrayNotification = trayNotification("Opening scheme-set download window...", imecore.TrayNotificationIconInfo)
	}

	go func() {
		defer func() {
			sharedSchemeSetDownloadState.mu.Lock()
			sharedSchemeSetDownloadState.running = false
			sharedSchemeSetDownloadState.mu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		pkg, err := schemeSetDownloadPromptFunc(ctx)
		if err != nil {
			log.Printf("scheme-set download failed: %v", err)
			ime.sendAsyncTrayNotification(trayNotification("Scheme-set download failed: "+shortErrorMessage(err), imecore.TrayNotificationIconError))
			return
		}
		displayName := strings.TrimSpace(pkg.Name)
		if displayName == "" {
			displayName = inferSchemeSetNameFromURL(pkg.URL)
		}
		debugLogf("scheme-set download started name=%q url=%q", displayName, pkg.URL)
		ime.sendAsyncTrayNotification(trayNotification("Scheme-set download started: "+displayName, imecore.TrayNotificationIconInfo))

		name, err := installSchemeSetPackage(ctx, pkg)
		if err != nil {
			log.Printf("scheme-set download failed: %v", err)
			ime.sendAsyncTrayNotification(trayNotification("Scheme-set download failed: "+shortErrorMessage(err), imecore.TrayNotificationIconError))
			return
		}
		debugLogf("scheme-set download completed name=%q", name)
		ime.sendAsyncTrayNotification(trayNotification("Scheme set downloaded; deploying: "+name, imecore.TrayNotificationIconInfo))

		ime.mu.Lock()
		activateResp := imecore.NewResponse(0, true)
		ok := ime.activateDownloadedSchemeSetLocked(name, req, activateResp)
		ime.mu.Unlock()
		if !ok {
			ime.sendAsyncTrayNotification(trayNotification("Scheme set downloaded, but deploy failed", imecore.TrayNotificationIconError))
			return
		}
		ime.sendAsyncTrayNotification(trayNotification("Scheme-set download and install succeeded: "+name, imecore.TrayNotificationIconInfo))
	}()

	return true
}

func (ime *IME) activateDownloadedSchemeSetLocked(name string, req *imecore.Request, resp *imecore.Response) bool {
	current := currentSchemeSetName()
	if !saveCurrentSchemeSetName(name) {
		return false
	}
	resp.TrayNotification = schemeSetTrayNotification("Switching scheme set...", imecore.TrayNotificationIconInfo)
	if ime.redeploy(req, resp) {
		ime.schemeSetVersion = bumpSchemeSetVersion()
		return true
	}
	_ = saveCurrentSchemeSetName(current)
	resp.TrayNotification = schemeSetTrayNotification("Scheme-set switch failed", imecore.TrayNotificationIconError)
	return false
}

func installSchemeSetPackage(ctx context.Context, pkg schemeSetDownloadPackage) (string, error) {
	if strings.TrimSpace(pkg.Name) == "" {
		pkg.Name = inferSchemeSetNameFromURL(pkg.URL)
	}
	downloadURLs := schemeSetDownloadURLs(pkg.URL)
	name, err := validateDownloadedSchemeSetName(pkg.Name)
	if err != nil {
		return "", err
	}
	if len(downloadURLs) == 0 {
		return "", errors.New("scheme-set download URL is empty")
	}

	archive, err := downloadFirstAvailableURL(ctx, downloadURLs)
	if err != nil {
		return "", err
	}
	if err := verifySchemeSetArchiveHash(archive, pkg.SHA256); err != nil {
		return "", err
	}

	root := moqiAppDataDir()
	if root == "" {
		return "", errors.New("APPDATA was not found")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("failed to create TypeDuck user directory: %w", err)
	}

	stagingDir, err := os.MkdirTemp(root, ".scheme-set-download-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	if err := extractSchemeSetZip(archive, stagingDir); err != nil {
		return "", err
	}
	if err := installExtractedSchemeSet(root, name, stagingDir); err != nil {
		return "", err
	}
	return name, nil
}

func downloadURL(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}
	resp, err := schemeSetDownloadHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	reader := io.LimitReader(resp.Body, limit+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read download content: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("download content is too large")
	}
	return data, nil
}

func validateDownloadedSchemeSetName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("scheme-set name is empty")
	}
	normalized := normalizeSchemeSetName(trimmed)
	if normalized != trimmed {
		return "", fmt.Errorf("invalid scheme-set name: %s", name)
	}
	return normalized, nil
}

func verifySchemeSetArchiveHash(data []byte, expected string) error {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return nil
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("scheme-set checksum verification failed")
	}
	return nil
}

func downloadFirstAvailableURL(ctx context.Context, rawURLs []string) ([]byte, error) {
	var lastErr error
	for _, rawURL := range rawURLs {
		archive, err := downloadURL(ctx, rawURL, maxSchemeSetArchiveBytes)
		if err == nil {
			return archive, nil
		}
		lastErr = err
		log.Printf("scheme-set download URL failed url=%s err=%v", rawURL, err)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("scheme-set download URL is empty")
}

func schemeSetDownloadURLs(rawURL string) []string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return []string{rawURL}
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return []string{rawURL}
	}
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	if owner == "" || repo == "" {
		return []string{rawURL}
	}
	if len(parts) >= 4 && parts[2] == "archive" {
		return []string{rawURL}
	}
	if len(parts) >= 4 && parts[2] == "tree" && parts[3] != "" {
		return []string{githubArchiveURL(owner, repo, parts[3])}
	}
	return []string{
		githubArchiveURL(owner, repo, "main"),
		githubArchiveURL(owner, repo, "master"),
	}
}

func normalizeSchemeSetDownloadURL(rawURL string) string {
	urls := schemeSetDownloadURLs(rawURL)
	if len(urls) == 0 {
		return rawURL
	}
	return urls[0]
}

func githubArchiveURL(owner, repo, branch string) string {
	return fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", owner, repo, branch)
}

func inferSchemeSetNameFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return defaultSchemeSetName
	}
	base := path.Base(parsed.Path)
	base = strings.TrimSuffix(base, ".zip")
	base = strings.TrimSuffix(base, ".tar")
	base = strings.TrimSuffix(base, ".gz")
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == "/" {
		base = parsed.Hostname()
	}
	base = strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		default:
			return r
		}
	}, base)
	base = strings.Trim(base, " .-")
	if base == "" {
		return defaultSchemeSetName
	}
	return base
}

func extractSchemeSetZip(data []byte, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("failed to open scheme-set archive: %w", err)
	}
	prefix := commonZipTopLevel(reader.File)
	var extracted uint64
	filesExtracted := 0
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
		}
		cleanName, ok := cleanArchivePath(name)
		if !ok {
			return fmt.Errorf("scheme-set archive contains an invalid path: %s", file.Name)
		}
		if cleanName == "" {
			continue
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(cleanName)), 0o755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}
		extracted += file.UncompressedSize64
		if extracted > maxSchemeSetExtractedBytes {
			return errors.New("extracted scheme-set content is too large")
		}
		if err := extractSchemeSetFile(file, dest, cleanName); err != nil {
			return err
		}
		filesExtracted++
	}
	if filesExtracted == 0 {
		return errors.New("scheme-set archive is empty")
	}
	return nil
}

func extractSchemeSetFile(file *zip.File, dest, cleanName string) error {
	target := filepath.Join(dest, filepath.FromSlash(cleanName))
	if !isPathInside(dest, target) {
		return fmt.Errorf("scheme-set archive contains an invalid path: %s", file.Name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to read archive file: %w", err)
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to extract file: %w", err)
	}
	return nil
}

func commonZipTopLevel(files []*zip.File) string {
	var prefix string
	for _, file := range files {
		name := strings.TrimLeft(strings.ReplaceAll(file.Name, "\\", "/"), "/")
		if name == "" {
			continue
		}
		parts := strings.SplitN(name, "/", 2)
		if len(parts) < 2 || parts[0] == "" || parts[0] == "." || parts[0] == ".." {
			return ""
		}
		if prefix == "" {
			prefix = parts[0] + "/"
			continue
		}
		if parts[0]+"/" != prefix {
			return ""
		}
	}
	return prefix
}

func cleanArchivePath(name string) (string, bool) {
	name = strings.TrimLeft(name, "/")
	if name == "" {
		return "", true
	}
	cleanName := path.Clean(name)
	if cleanName == "." {
		return "", true
	}
	if cleanName == ".." || strings.HasPrefix(cleanName, "../") || path.IsAbs(cleanName) {
		return "", false
	}
	return cleanName, true
}

func installExtractedSchemeSet(root, name, stagingDir string) error {
	targetDir := filepath.Join(root, name)
	if !isPathInside(root, targetDir) {
		return fmt.Errorf("invalid scheme-set install directory: %s", name)
	}

	backupDir := ""
	if _, err := os.Stat(targetDir); err == nil {
		backupDir = filepath.Join(root, fmt.Sprintf(".%s.backup-%d", name, time.Now().UnixNano()))
		if err := os.Rename(targetDir, backupDir); err != nil {
			return fmt.Errorf("failed to back up existing scheme set: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect existing scheme set: %w", err)
	}

	if err := os.Rename(stagingDir, targetDir); err != nil {
		if backupDir != "" {
			_ = os.Rename(backupDir, targetDir)
		}
		return fmt.Errorf("failed to install scheme set: %w", err)
	}
	if backupDir != "" {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

func isPathInside(root, target string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}

func shortErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	runes := []rune(msg)
	if len(runes) <= 48 {
		return msg
	}
	return string(runes[:48]) + "..."
}
