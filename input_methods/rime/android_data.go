//go:build android

package rime

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed data/**
var androidRimeData embed.FS

var (
	androidDirsOnce  sync.Once
	androidSharedDir string
	androidUserDir   string
	androidDirsOK    bool
)

func androidRimeDirs() (sharedDir string, userDir string, ok bool) {
	androidDirsOnce.Do(func() {
		baseDir, err := os.UserConfigDir()
		if err != nil || strings.TrimSpace(baseDir) == "" {
			baseDir = os.TempDir()
		}
		if strings.TrimSpace(baseDir) == "" {
			log.Printf("Android RIME 数据目录不可用: %v", err)
			return
		}

		root := filepath.Join(baseDir, APP)
		androidSharedDir = filepath.Join(root, "rime_shared")
		androidUserDir = filepath.Join(root, currentSchemeSetName())
		if err := extractAndroidRimeData(androidSharedDir); err != nil {
			log.Printf("解压 Android RIME 数据失败: %v", err)
			return
		}
		if err := os.MkdirAll(androidUserDir, 0o700); err != nil {
			log.Printf("创建 Android RIME 用户目录失败: %v", err)
			return
		}
		androidDirsOK = true
	})
	return androidSharedDir, androidUserDir, androidDirsOK
}

func extractAndroidRimeData(dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(androidRimeData, "data", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("data", path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		content, err := androidRimeData.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o644)
	})
}
