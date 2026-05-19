package cloudclipboard

import "testing"

func TestContentMD5(t *testing.T) {
	const text = "hello"
	const want = "5d41402abc4b2a76b9719d911017c592"
	if got := ContentMD5(text); got != want {
		t.Fatalf("ContentMD5(%q) = %q, want %q", text, got, want)
	}
}

func TestBuildFilename(t *testing.T) {
	if got := BuildFilename("hello"); got != "5d41402abc4b2a76b9719d911017c592.txt" {
		t.Fatalf("unexpected filename: %s", got)
	}
}

func TestNormalizeDir(t *testing.T) {
	if got := NormalizeDir("moqi-input-method"); got != "/moqi-input-method/" {
		t.Fatalf("NormalizeDir = %q", got)
	}
}
