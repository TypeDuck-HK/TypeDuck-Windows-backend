package mobilebridge

import "testing"

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
