package cloudclipboard

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
	"unicode"
)

const (
	minTextLength = 2
	maxTextLength = 32_768
)

func ContentMD5(text string) string {
	sum := md5.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}

func BuildFilename(text string) string {
	return ContentMD5(text) + ".txt"
}

func IsUploadableText(text string) bool {
	if len(text) < minTextLength || len(text) > maxTextLength {
		return false
	}
	if strings.TrimSpace(text) == "" {
		return false
	}
	if looksLikeMaskedSecret(text) {
		return false
	}
	return true
}

func ShouldUpload(
	cfg Config,
	text string,
	isApplyingRemote bool,
	lastUploadedHash string,
) bool {
	if !cfg.Enabled || !cfg.IsComplete() {
		return false
	}
	if isApplyingRemote {
		return false
	}
	if !IsUploadableText(text) {
		return false
	}
	md5 := ContentMD5(text)
	if md5 == lastUploadedHash {
		return false
	}
	return true
}

func looksLikeMaskedSecret(text string) bool {
	if len(text) < 4 {
		return false
	}
	maskChars := map[rune]bool{'*': true, '•': true, '●': true, '·': true}
	masked := 0
	for _, r := range text {
		if maskChars[r] {
			masked++
		}
	}
	return masked >= len([]rune(text))*8/10
}

func FormatPreview(text string, maxLength int) string {
	if text == "" {
		return "（空）"
	}
	singleLine := strings.Join(strings.Fields(text), " ")
	if len([]rune(singleLine)) <= maxLength {
		return singleLine
	}
	runes := []rune(singleLine)
	return string(runes[:maxLength]) + "…"
}

func IsClipFileName(name string) bool {
	if !strings.HasSuffix(strings.ToLower(name), ".txt") {
		return false
	}
	if strings.Contains(name, "/") {
		return false
	}
	base := name[:len(name)-4]
	if len(base) == 32 && isHexLower(base) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(name), "clip_")
}

func IsUserDictSnapshotFileName(name string) bool {
	if !strings.HasSuffix(strings.ToLower(name), ".userdb.txt") {
		return false
	}
	if strings.ContainsAny(name, `/\:`) {
		return false
	}
	base := strings.TrimSuffix(name, ".userdb.txt")
	return base != "" && base != "." && base != ".."
}

func IsSyncDeviceDirName(name string) bool {
	if name == "" || name == "." || name == ".." || len(name) > 128 {
		return false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func IsSchemeSetDirName(name string) bool {
	if name == "" || name == "." || name == ".." || len([]rune(name)) > 128 {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == '/' || r == '\\' || r == ':' {
			return false
		}
	}
	return true
}

func isHexLower(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func IsSensitivePlaceholder(text string) bool {
	return !IsUploadableText(strings.TrimSpace(text))
}

// NormalizePlainText trims clipboard text for upload.
func NormalizePlainText(text string) string {
	return strings.TrimSpace(text)
}

// MaskRunesForDebug returns rune count only (avoid logging content).
func MaskRunesForDebug(text string) int {
	return len([]rune(text))
}
