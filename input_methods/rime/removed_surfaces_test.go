package rime

import (
	"strings"
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

func TestTypeDuckV1MenuLabelsAreBilingualTraditional(t *testing.T) {
	ime := newIsolatedTestIME(t)
	items := flattenMenuItemsForRemovedSurfaceTest(ime.buildMenu())

	simplifiedOnlyText := []string{
		"切换方案集",
		"输入方案(&I)",
		"打开超级简拼",
		"刷新配置(&R)",
		"输入状态共享",
		"外观(&A)",
		"候选排列",
		"竖排",
		"横排",
		"每行候选数",
		"候选间距",
		"总候选数量",
		"字体大小",
		"候选文字字体",
		"注释文字大小",
		"注释文字字体",
		"候选框背景",
		"高亮颜色",
		"字体颜色",
		"高亮文字颜色",
		"导入皮肤",
		"输入设置",
		"分号键次选",
		"打开文件夹(&O)",
		"用户文件夹",
		"共享文件夹",
		"同步文件夹",
		"日志文件夹",
		"帮助文档(&H)",
		"参加讨论(&J)",
	}
	for _, text := range simplifiedOnlyText {
		if _, ok := items.byText[text]; ok {
			t.Fatalf("Simplified-only menu label %q is still visible", text)
		}
	}

	requiredBilingual := []string{
		"切換方案集 / Switch Scheme Set",
		"輸入方案(&I) / Input Schema",
		"外觀(&A) / Appearance",
		"輸入設定 / Input Settings",
		"說明文件(&H) / Help",
	}
	for _, text := range requiredBilingual {
		if _, ok := items.byText[text]; !ok {
			t.Fatalf("expected bilingual Traditional/English menu label %q, got %#v", text, items.byText)
		}
	}

	for text := range items.byText {
		if strings.ContainsAny(text, "切换输入设置用户文件夹日志帮助参加讨论颜色字体候选数量总") &&
			!strings.Contains(text, " / ") &&
			!containsASCIIWord(text) {
			t.Fatalf("menu label %q contains Simplified wording without bilingual English", text)
		}
	}
}

func containsASCIIWord(text string) bool {
	for _, ch := range text {
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			return true
		}
	}
	return false
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
