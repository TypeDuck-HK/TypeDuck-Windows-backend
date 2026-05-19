package cloudclipboard

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	debounceMs        = 500
	maxDisplayItems   = 40
	previewMaxLength  = 240
	remoteClipClearMs = 300
)

// RemoteClipClearDuration is how long uploads are suppressed after pasting a remote clip.
func RemoteClipClearDuration() time.Duration {
	return remoteClipClearMs * time.Millisecond
}

type DisplayItem struct {
	Name         string
	LastModified int64
	Preview      string
	FullText     string
}

type Sync struct {
	mu sync.Mutex

	cfg              Config
	client           *Client
	lastUploadAt     time.Time
	lastUploadedHash string
	isUploadInFlight bool
	pendingText      string
	debounceTimer    *time.Timer
	applyingRemote   bool
	applyingUntil    time.Time
}

func NewSync(cfg Config) *Sync {
	s := &Sync{cfg: cfg}
	s.reloadClient()
	return s
}

func (s *Sync) ReloadConfig(cfg Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	s.reloadClient()
}

func (s *Sync) reloadClient() {
	if s.cfg.Enabled && s.cfg.IsComplete() && IsAllowedBaseURL(s.cfg.BaseURL) {
		s.client = NewClient(s.cfg)
	} else {
		s.client = nil
	}
}

func (s *Sync) LastUploadedHash() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUploadedHash
}

func (s *Sync) SetApplyingRemote(active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyingRemote = active
	if active {
		s.applyingUntil = time.Now().Add(remoteClipClearMs * time.Millisecond)
	} else {
		s.applyingUntil = time.Time{}
	}
}

func (s *Sync) isApplyingRemote() bool {
	if s.applyingRemote && time.Now().Before(s.applyingUntil) {
		return true
	}
	if s.applyingRemote && !time.Now().Before(s.applyingUntil) {
		s.applyingRemote = false
	}
	return false
}

func (s *Sync) ScheduleUpload(text string) {
	s.mu.Lock()
	if s.client == nil || !s.cfg.Enabled {
		s.mu.Unlock()
		return
	}
	s.pendingText = text
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
	}
	s.debounceTimer = time.AfterFunc(debounceMs*time.Millisecond, func() {
		s.mu.Lock()
		toUpload := s.pendingText
		s.pendingText = ""
		s.mu.Unlock()
		if toUpload != "" {
			go func(payload string) { _ = s.performUpload(payload) }(toUpload)
		}
	})
	s.mu.Unlock()
}

func (s *Sync) performUpload(text string) error {
	s.mu.Lock()
	if s.client == nil || !s.cfg.Enabled {
		s.mu.Unlock()
		return nil
	}
	if s.isUploadInFlight {
		s.pendingText = text
		s.mu.Unlock()
		return nil
	}
	if s.isApplyingRemote() {
		s.mu.Unlock()
		return nil
	}
	now := time.Now()
	elapsed := now.Sub(s.lastUploadAt)
	minInterval := time.Duration(s.cfg.MinIntervalMs()) * time.Millisecond
	if elapsed < minInterval {
		delay := minInterval - elapsed + 50*time.Millisecond
		pending := text
		s.mu.Unlock()
		time.AfterFunc(delay, func() {
			_ = s.performUpload(pending)
		})
		return nil
	}
	md5 := ContentMD5(text)
	if md5 == s.lastUploadedHash {
		s.mu.Unlock()
		return nil
	}
	if !ShouldUpload(s.cfg, text, false, s.lastUploadedHash) {
		s.mu.Unlock()
		return nil
	}
	client := s.client
	s.isUploadInFlight = true
	s.mu.Unlock()

	filename := BuildFilename(text)
	err := client.UploadClip(filename, text)

	s.mu.Lock()
	s.isUploadInFlight = false
	if err != nil {
		s.lastUploadAt = time.Now()
		retry := s.pendingText
		s.mu.Unlock()
		if retry != "" && retry != text {
			go func(payload string) { _ = s.performUpload(payload) }(retry)
		}
		return err
	}
	s.lastUploadedHash = md5
	s.lastUploadAt = time.Now()
	retry := s.pendingText
	s.mu.Unlock()
	if retry != "" && retry != text {
		go func(payload string) { _ = s.performUpload(payload) }(retry)
	}
	return nil
}

func (s *Sync) UploadFromRequest(text string) {
	text = NormalizePlainText(text)
	if text == "" {
		return
	}
	s.mu.Lock()
	applying := s.isApplyingRemote()
	hash := s.lastUploadedHash
	cfg := s.cfg
	client := s.client
	s.mu.Unlock()
	if client == nil || !cfg.Enabled {
		return
	}
	if !ShouldUpload(cfg, text, applying, hash) {
		return
	}
	s.ScheduleUpload(text)
}

func (s *Sync) TestConnection() error {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return fmt.Errorf("云剪贴板未配置完整")
	}
	return client.TestConnection()
}

func (s *Sync) ListClipsForDisplay(ctx context.Context) ([]DisplayItem, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("云剪贴板未配置完整")
	}
	entries, err := client.ListClips()
	if err != nil {
		return nil, err
	}
	if len(entries) > maxDisplayItems {
		entries = entries[:maxDisplayItems]
	}
	items := make([]DisplayItem, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}
		body, err := client.DownloadClip(entry.Name)
		if err != nil {
			continue
		}
		body = NormalizePlainText(body)
		items = append(items, DisplayItem{
			Name:         entry.Name,
			LastModified: entry.LastModified,
			Preview:      FormatPreview(body, previewMaxLength),
			FullText:     body,
		})
	}
	return items, nil
}

func (s *Sync) DownloadClip(name string) (string, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return "", fmt.Errorf("云剪贴板未配置完整")
	}
	return client.DownloadClip(name)
}

func (s *Sync) DeleteClip(name string) error {
	if !IsClipFileName(name) {
		return fmt.Errorf("非法云剪贴板文件名")
	}
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return fmt.Errorf("云剪贴板未配置完整")
	}
	return client.DeleteClip(name)
}

// ClientOrError returns the WebDAV client when configured.
func (s *Sync) ClientOrError() (*Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		if !IsAllowedBaseURL(s.cfg.BaseURL) {
			return nil, fmt.Errorf("%s", RejectionMessage())
		}
		return nil, fmt.Errorf("云剪贴板未配置完整")
	}
	return s.client, nil
}
