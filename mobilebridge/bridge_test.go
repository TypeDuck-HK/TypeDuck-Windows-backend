package mobilebridge

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSessionTypesAndSelectsCandidate(t *testing.T) {
	session := NewSession(GUIDMoqi)
	if resp := session.Init(); !resp.Success {
		t.Fatalf("init failed: %s", resp.Error)
	}
	if resp := session.Activate(); !resp.Success {
		t.Fatalf("activate failed: %s", resp.Error)
	}

	for _, ch := range "nihao" {
		resp := session.KeyDown(int(ch), int(ch))
		if !resp.Success {
			t.Fatalf("key %q failed: %s", ch, resp.Error)
		}
	}

	resp := session.KeyDown(int('1'), int('1'))
	if resp.CommitString != "你好" {
		t.Fatalf("expected first candidate committed, got %q", resp.CommitString)
	}
}

func TestSessionSelectCandidateByIndex(t *testing.T) {
	session := NewSession(GUIDMoqi)
	if resp := session.Init(); !resp.Success {
		t.Fatalf("init failed: %s", resp.Error)
	}

	for _, ch := range "nihao" {
		session.KeyDown(int(ch), int(ch))
	}

	resp := session.SelectCandidate(1)
	if resp.CommitString != "您好" {
		t.Fatalf("expected second candidate committed, got %q", resp.CommitString)
	}
}

func TestSessionBackspaceUpdatesComposition(t *testing.T) {
	session := NewSession(GUIDMoqi)
	if resp := session.Init(); !resp.Success {
		t.Fatalf("init failed: %s", resp.Error)
	}

	session.KeyDown(int('n'), int('n'))
	session.KeyDown(int('i'), int('i'))
	resp := session.KeyDown(0x08, 0)
	if resp.CompositionString != "n" {
		t.Fatalf("expected composition n after backspace, got %q", resp.CompositionString)
	}
}

func TestRimeT9ReplaySelectedPinyin(t *testing.T) {
	if runtime.GOOS != "android" {
		t.Skip("mobilebridge Rime T9 replay needs Android embedded Rime data")
	}
	androidDataDir := filepath.Join(`C:\Program Files (x86)\MoqiIM\moqi-ime`)
	if info, err := os.Stat(androidDataDir); err != nil || !info.IsDir() {
		t.Skipf("android data dir unavailable: %q err=%v", androidDataDir, err)
	}
	SetAndroidDataDir(androidDataDir)

	session := NewSession(GUIDRime)
	if resp := session.Init(); !resp.Success {
		t.Fatalf("init failed: %s", resp.Error)
	}
	if resp := session.Activate(); !resp.Success {
		t.Fatalf("activate failed: %s", resp.Error)
	}
	if resp := session.SelectSchema("rime_frost_t9"); !resp.Success {
		t.Fatalf("select t9 schema failed: %s", resp.Error)
	}

	for _, ch := range "6442662" {
		resp := session.KeyDown(int(ch), int(ch))
		if !resp.Success {
			t.Fatalf("digit %q failed: %s", ch, resp.Error)
		}
	}
	session.KeyDown(0x1B, 0)

	var resp *MobileResponse
	for _, ch := range "ni'hao'ma" {
		resp = session.KeyDown(int(ch), int(ch))
		if !resp.Success {
			t.Fatalf("replay key %q failed: %s", ch, resp.Error)
		}
	}
	if resp == nil || resp.CandidateList.Len() == 0 {
		t.Fatalf("expected candidates after replay, got %#v", resp)
	}
	if got := resp.CandidateList.Get(0); got != "你好吗" {
		t.Fatalf("expected first candidate 你好吗 after replay, got %q", got)
	}
}
