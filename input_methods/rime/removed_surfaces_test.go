package rime

import (
	"testing"

	"github.com/gaboolic/moqi-ime/imecore"
)

func TestTypeDuckV1MenuOmitsRemovedBackendSurfaces(t *testing.T) {
	ime := newIsolatedTestIME(t)
	items := flattenMenuItemsForRemovedSurfaceTest(ime.buildMenu())

	bannedText := []string{
		"下载方案集(&D)",
		"更新配置(&P)",
		"打开置顶短语",
		"自动插入成对符号",
		"打开成对符号设置",
		"WebDAV 配置...",
		"测试 WebDAV",
		"启用云剪贴板",
		"云剪贴板设置...",
	}
	for _, text := range bannedText {
		if _, ok := items.byText[text]; ok {
			t.Fatalf("removed v1 menu surface %q is still visible", text)
		}
	}

	bannedIDs := []int{
		ID_DOWNLOAD_SCHEME_SET,
		ID_UPDATE_CONFIG,
		ID_OPEN_CUSTOM_PHRASE,
		ID_INPUT_AUTO_PAIR_QUOTES,
		ID_OPEN_AUTO_PAIR_SYMBOLS,
		ID_WEBDAV_SETTINGS,
		ID_CLOUD_CLIPBOARD_TEST,
		ID_CLOUD_CLIPBOARD_ENABLED,
		ID_CLOUD_CLIPBOARD_SETTINGS,
	}
	for _, id := range bannedIDs {
		if _, ok := items.byID[id]; ok {
			t.Fatalf("removed v1 menu command id %d is still visible", id)
		}
	}
}

func TestTypeDuckV1RemovedBackendCommandsAreUnhandled(t *testing.T) {
	ime := newIsolatedTestIME(t)

	bannedIDs := []int{
		ID_DOWNLOAD_SCHEME_SET,
		ID_UPDATE_CONFIG,
		ID_OPEN_CUSTOM_PHRASE,
		ID_INPUT_AUTO_PAIR_QUOTES,
		ID_OPEN_AUTO_PAIR_SYMBOLS,
		ID_WEBDAV_SETTINGS,
		ID_CLOUD_CLIPBOARD_TEST,
		ID_CLOUD_CLIPBOARD_ENABLED,
		ID_CLOUD_CLIPBOARD_SETTINGS,
	}
	for _, id := range bannedIDs {
		resp := ime.onCommand(&imecore.Request{
			SeqNum: 9000 + id,
			ID:     imecore.FlexibleID{Int: id, IsInt: true},
		}, imecore.NewResponse(9000+id, true))
		if resp.ReturnValue != 0 {
			t.Fatalf("removed v1 command id %d returned %d", id, resp.ReturnValue)
		}
	}
}

func TestTypeDuckV1CustomizeUIOmitsAutoPairPayload(t *testing.T) {
	ime := newIsolatedTestIME(t)
	customizeUI := ime.customizeUIMap()

	if _, ok := customizeUI["autoPairQuotes"]; ok {
		t.Fatal("autoPairQuotes should not be sent in TypeDuck v1 CustomizeUI")
	}
	if _, ok := customizeUI["autoPairRules"]; ok {
		t.Fatal("autoPairRules should not be sent in TypeDuck v1 CustomizeUI")
	}
}

func TestTypeDuckV1OnMenuDoesNotExposeLegacyBackendMenu(t *testing.T) {
	ime := newIsolatedTestIME(t)

	for _, id := range []string{"settings", "windows-mode-icon", "other"} {
		resp := ime.onMenu(&imecore.Request{
			SeqNum: 9100,
			ID:     imecore.FlexibleID{String: id},
		}, imecore.NewResponse(9100, true))
		if resp.ReturnValue != 0 {
			t.Fatalf("onMenu %q should be unreachable in TypeDuck v1, got ReturnValue=%d", id, resp.ReturnValue)
		}
		items, ok := resp.ReturnData.([]map[string]interface{})
		if !ok {
			t.Fatalf("onMenu %q returned unexpected payload type %#v", id, resp.ReturnData)
		}
		if len(items) != 0 {
			t.Fatalf("onMenu %q exposed legacy backend menu items: %#v", id, items)
		}
	}
}

type removedSurfaceMenuIndex struct {
	byText map[string]struct{}
	byID   map[int]struct{}
}

func flattenMenuItemsForRemovedSurfaceTest(items []map[string]interface{}) removedSurfaceMenuIndex {
	index := removedSurfaceMenuIndex{
		byText: map[string]struct{}{},
		byID:   map[int]struct{}{},
	}
	var walk func([]map[string]interface{})
	walk = func(entries []map[string]interface{}) {
		for _, entry := range entries {
			if text, ok := entry["text"].(string); ok {
				index.byText[text] = struct{}{}
			}
			if id, ok := entry["id"].(int); ok {
				index.byID[id] = struct{}{}
			}
			if submenu, ok := entry["submenu"].([]map[string]interface{}); ok {
				walk(submenu)
			}
		}
	}
	walk(items)
	return index
}
