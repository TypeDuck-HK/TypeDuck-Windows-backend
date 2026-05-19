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

func TestIsUserDictSnapshotFileName(t *testing.T) {
	valid := []string{"rime_frost.userdb.txt", "luna_pinyin.userdb.txt"}
	for _, name := range valid {
		if !IsUserDictSnapshotFileName(name) {
			t.Fatalf("expected valid userdb snapshot name: %s", name)
		}
	}

	invalid := []string{"", ".userdb.txt", "rime.userdb", "../rime.userdb.txt", `a\b.userdb.txt`, "rime.txt"}
	for _, name := range invalid {
		if IsUserDictSnapshotFileName(name) {
			t.Fatalf("expected invalid userdb snapshot name: %s", name)
		}
	}
}

func TestIsSyncDeviceDirName(t *testing.T) {
	if !IsSyncDeviceDirName("8f7d8ff2-1d8e-4b8d-9db4-test") {
		t.Fatal("expected UUID-like device id to be valid")
	}
	invalid := []string{"", ".", "..", "../device", `device\name`, "device:name", "device name"}
	for _, name := range invalid {
		if IsSyncDeviceDirName(name) {
			t.Fatalf("expected invalid device dir name: %s", name)
		}
	}
}

func TestIsSchemeSetDirName(t *testing.T) {
	valid := []string{"Rime", "Work", "小鹤双拼"}
	for _, name := range valid {
		if !IsSchemeSetDirName(name) {
			t.Fatalf("expected valid scheme set dir name: %s", name)
		}
	}

	invalid := []string{"", ".", "..", "../Rime", `Work\Rime`, "Work:Rime"}
	for _, name := range invalid {
		if IsSchemeSetDirName(name) {
			t.Fatalf("expected invalid scheme set dir name: %s", name)
		}
	}
}
