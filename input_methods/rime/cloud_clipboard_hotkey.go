package rime

import (
	"fmt"
	"strings"

	"github.com/gaboolic/moqi-ime/imecore"
)

const (
	cloudClipboardListPreservedKeyGUID = "{9d9f5a64-1a18-47f0-9a4f-7cbf2f5a44d9}"
	cloudClipboardDefaultListHotkey    = "Ctrl+0"
	cloudClipboardOldShiftZeroHotkey   = "Ctrl+Shift+0"
	cloudClipboardOldShiftVHotkey      = "Ctrl+Shift+V"

	tfModAlt     = 0x0001
	tfModControl = 0x0002
	tfModShift   = 0x0004
)

type cloudClipboardHotkey struct {
	Hotkey  string
	KeyCode int
	Ctrl    bool
	Alt     bool
	Shift   bool
}

func parseCloudClipboardHotkey(hotkey string) (cloudClipboardHotkey, error) {
	parts := strings.Split(hotkey, "+")
	action := cloudClipboardHotkey{Hotkey: strings.TrimSpace(hotkey)}
	if len(parts) == 0 {
		return action, fmt.Errorf("empty hotkey")
	}
	var mainKey string
	for _, part := range parts {
		token := strings.ToUpper(strings.TrimSpace(part))
		if token == "" {
			continue
		}
		switch token {
		case "CTRL", "CONTROL":
			action.Ctrl = true
		case "ALT", "MENU":
			action.Alt = true
		case "SHIFT":
			action.Shift = true
		default:
			if mainKey != "" {
				return action, fmt.Errorf("hotkey %q has multiple main keys", hotkey)
			}
			mainKey = token
		}
	}
	if mainKey == "" {
		return action, fmt.Errorf("hotkey %q is missing main key", hotkey)
	}
	if len(mainKey) == 1 {
		ch := mainKey[0]
		if ch >= 'A' && ch <= 'Z' {
			action.KeyCode = int(ch)
			return action, nil
		}
		if ch >= '0' && ch <= '9' {
			action.KeyCode = int(ch)
			return action, nil
		}
	}
	return action, fmt.Errorf("unsupported hotkey key %q", mainKey)
}

func (action cloudClipboardHotkey) matches(req *imecore.Request) bool {
	if req == nil || req.KeyCode != action.KeyCode {
		return false
	}
	return action.modifiersMatch(req)
}

func (action cloudClipboardHotkey) matchesKeyUp(req *imecore.Request) bool {
	if req == nil || req.KeyCode != action.KeyCode {
		return false
	}
	return action.modifiersMatch(req)
}

func (action cloudClipboardHotkey) modifiersMatch(req *imecore.Request) bool {
	return action.ctrlDown(req) == action.Ctrl &&
		action.altDown(req) == action.Alt &&
		action.shiftDown(req) == action.Shift
}

func (action cloudClipboardHotkey) ctrlDown(req *imecore.Request) bool {
	return req.KeyStates.IsKeyDown(vkControl) ||
		req.KeyStates.IsKeyDown(vkLControl) ||
		req.KeyStates.IsKeyDown(vkRControl) ||
		req.KeyCode == vkControl ||
		req.KeyCode == vkLControl ||
		req.KeyCode == vkRControl
}

func (action cloudClipboardHotkey) altDown(req *imecore.Request) bool {
	return req.KeyStates.IsKeyDown(vkMenu) ||
		req.KeyStates.IsKeyDown(vkLMenu) ||
		req.KeyStates.IsKeyDown(vkRMenu) ||
		req.KeyCode == vkMenu ||
		req.KeyCode == vkLMenu ||
		req.KeyCode == vkRMenu
}

func (action cloudClipboardHotkey) shiftDown(req *imecore.Request) bool {
	return req.KeyStates.IsKeyDown(vkShift) ||
		req.KeyStates.IsKeyDown(vkLShift) ||
		req.KeyStates.IsKeyDown(vkRShift) ||
		req.KeyCode == vkShift ||
		req.KeyCode == vkLShift ||
		req.KeyCode == vkRShift
}

func (action cloudClipboardHotkey) preservedModifiers() uint32 {
	var modifiers uint32
	if action.Alt {
		modifiers |= tfModAlt
	}
	if action.Ctrl {
		modifiers |= tfModControl
	}
	if action.Shift {
		modifiers |= tfModShift
	}
	return modifiers
}

func loadCloudClipboardListHotkey() cloudClipboardHotkey {
	cfg := loadCloudClipboardConfig()
	action, err := parseCloudClipboardHotkey(cfg.ListHotkey)
	if err != nil {
		action, _ = parseCloudClipboardHotkey(cloudClipboardDefaultListHotkey)
	}
	return action
}

func cloudClipboardListPreservedKeyInfo() imecore.PreservedKeyInfo {
	action := loadCloudClipboardListHotkey()
	return imecore.PreservedKeyInfo{
		KeyCode:   uint32(action.KeyCode),
		Modifiers: action.preservedModifiers(),
		GUID:      cloudClipboardListPreservedKeyGUID,
	}
}
