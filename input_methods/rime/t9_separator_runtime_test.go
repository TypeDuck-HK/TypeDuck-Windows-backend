//go:build windows

package rime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRealRimeT9SeparatorProbe(t *testing.T) {
	dataDir := filepath.Join(`C:\Program Files (x86)\MoqiIM\moqi-ime`, "input_methods", "rime", "data")
	userDir := filepath.Join(`C:\Users\gbl\AppData\Roaming`, APP, "Rime")
	if info, err := os.Stat(dataDir); err != nil || !info.IsDir() {
		t.Skipf("data dir unavailable: %q err=%v", dataDir, err)
	}
	if info, err := os.Stat(userDir); err != nil || !info.IsDir() {
		t.Skipf("user dir unavailable: %q err=%v", userDir, err)
	}

	if !RimeInit(dataDir, userDir, APP, APP_VERSION, false) {
		t.Fatal("RimeInit failed")
	}
	defer Finalize()

	sessionID, ok := StartSession()
	if !ok || sessionID == 0 {
		t.Fatal("StartSession failed")
	}
	defer EndSession(sessionID)

	if !SelectSchema(sessionID, "rime_frost_t9") {
		t.Fatal("SelectSchema(rime_frost_t9) failed")
	}
	SetOption(sessionID, "ascii_mode", false)

	tests := []struct {
		input          string
		wantPreedit    string
		wantFirst      string
	}{
		{input: "9426", wantPreedit: "9426"},
		{input: "94126", wantPreedit: "94'26"},
		{input: "94'26", wantPreedit: "94'26"},
	}
	for _, tc := range tests {
		input := tc.input
		t.Run(input, func(t *testing.T) {
			ClearComposition(sessionID)
			for _, key := range input {
				if !ProcessKey(sessionID, int(key), 0) {
					t.Logf("ProcessKey returned false for %q in %q", key, input)
				}
			}

			rawInput := GetInput(sessionID)
			t.Logf("input=%q rawInput=%q schema=%q", input, rawInput, GetCurrentSchema(sessionID))
			if composition, ok := GetComposition(sessionID); ok {
				t.Logf("composition preedit=%q", composition.Preedit)
				if tc.wantPreedit != "" && composition.Preedit != tc.wantPreedit {
					t.Fatalf("expected preedit %q for %q, got %q", tc.wantPreedit, input, composition.Preedit)
				}
			}
			menu, ok := GetMenu(sessionID)
			if !ok {
				t.Fatalf("GetMenu failed for %q", input)
			}
			if tc.wantFirst != "" {
				if len(menu.Candidates) == 0 {
					t.Fatalf("expected candidates for %q", input)
				}
				if menu.Candidates[0].Text != tc.wantFirst {
					t.Fatalf("expected first candidate %q for %q, got %q", tc.wantFirst, input, menu.Candidates[0].Text)
				}
			}
			for i, candidate := range menu.Candidates {
				t.Logf("candidate[%d]=text:%q comment:%q", i, candidate.Text, candidate.Comment)
			}
		})
	}
}
