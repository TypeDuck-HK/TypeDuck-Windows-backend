package rime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ensureTestThemeRegistry(t *testing.T) {
	t.Helper()
	resetThemeRegistryForTest()
	if err := reloadThemeRegistry(); err != nil {
		t.Fatalf("reloadThemeRegistry: %v", err)
	}
	if len(listThemes()) == 0 {
		t.Fatalf("expected builtin themes in registry, builtin path=%q", builtinAppearanceThemesPath())
	}
}

func themeCommandForTheme(t *testing.T, themeID string) int {
	t.Helper()
	ensureTestThemeRegistry(t)
	themes := listThemes()
	for index, theme := range themes {
		if theme.ID == normalizeThemeID(themeID) {
			return themeCommandID(index)
		}
	}
	t.Fatalf("theme %q not found (%d themes loaded)", themeID, len(themes))
	return -1
}

func TestBuiltinThemeRegistryLoads(t *testing.T) {
	ensureTestThemeRegistry(t)
	themes := listThemes()
	if len(themes) != 2 {
		t.Fatalf("expected light/dark builtin themes, got %d", len(themes))
	}
	theme, ok := lookupRegisteredTheme("light")
	if !ok {
		t.Fatal("expected light theme")
	}
	if theme.Label != "淺色 Light" {
		t.Fatalf("unexpected label: %q", theme.Label)
	}
	if theme.Source != "" {
		t.Fatalf("builtin themes must not expose development-source metadata, got %q", theme.Source)
	}
}

func TestPaletteAppearanceConfigNameAvoidsAbstractTerminology(t *testing.T) {
	source, err := os.ReadFile("appearance_themes.go")
	if err != nil {
		t.Fatal(err)
	}
	oldName := "sema" + "nticPaletteAppearanceConfig"
	if strings.Contains(string(source), oldName) {
		t.Fatal("palette appearance mapping should not use the old misleading name")
	}
}

func TestAppearanceThemeMenuItems(t *testing.T) {
	ensureTestThemeRegistry(t)
	ime := newTestIME()
	items := ime.themeMenuItems()
	if len(items) != 2 {
		t.Fatalf("expected light/dark theme menu items, got %d", len(items))
	}
	if items[0]["id"].(int) != ID_APPEARANCE_THEME_BASE {
		t.Fatalf("expected first theme command id %d, got %v", ID_APPEARANCE_THEME_BASE, items[0]["id"])
	}
}

func TestParseWeaselSkinAllSchemes(t *testing.T) {
	data := []byte(`
style:
  color_scheme: aqua
  font_face: Microsoft YaHei
  font_point: 14
preset_color_schemes:
  aqua:
    name: 碧水／Aqua
    back_color: 0xeceeee
    hilited_candidate_back_color: 0xfa3a0a
  google:
    name: Google
    back_color: 0xffffff
    hilited_candidate_back_color: 0xce7539
`)
	activeID, themes, err := parseWeaselSkinFromBytes(data)
	if err != nil {
		t.Fatalf("parseWeaselSkinFromBytes: %v", err)
	}
	if activeID != "aqua" {
		t.Fatalf("expected active aqua, got %q", activeID)
	}
	if len(themes) != 2 {
		t.Fatalf("expected 2 themes, got %d", len(themes))
	}
	var aquaTheme ThemeDefinition
	for _, theme := range themes {
		if theme.ID == "aqua" {
			aquaTheme = theme
			break
		}
	}
	if aquaTheme.ID == "" {
		t.Fatal("expected aqua theme")
	}
	if aquaTheme.Label != "碧水／Aqua" {
		t.Fatalf("unexpected label: %q", aquaTheme.Label)
	}
	if aquaTheme.Appearance.CandidateHighlightColor == nil || *aquaTheme.Appearance.CandidateHighlightColor != "#0a3afa" {
		t.Fatalf("unexpected aqua highlight: %#v", aquaTheme.Appearance.CandidateHighlightColor)
	}
}

func TestImportWeaselSkinRegistersThemes(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	resetSharedAppearanceConfigForTest()
	resetThemeRegistryForTest()

	data := []byte(`
style:
  color_scheme: aqua
  font_point: 14
preset_color_schemes:
  aqua:
    back_color: 0xeceeee
    hilited_candidate_back_color: 0xfa3a0a
  google:
    back_color: 0xffffff
    hilited_candidate_back_color: 0xce7539
`)
	path := filepath.Join(t.TempDir(), "weasel.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ime := newTestIME()
	if err := ime.importAppearanceSkinFromFile(path); err != nil {
		t.Fatalf("import: %v", err)
	}
	if ime.style.CandidateTheme != "aqua" {
		t.Fatalf("expected active theme aqua, got %q", ime.style.CandidateTheme)
	}
	if ime.style.CandidateHighlightColor != "#0a3afa" {
		t.Fatalf("expected blue highlight, got %q", ime.style.CandidateHighlightColor)
	}

	themes := listThemes()
	imported := 0
	for _, theme := range themes {
		if theme.Source == "imported" {
			imported++
		}
	}
	if imported < 2 {
		t.Fatalf("expected imported themes in registry, got %d imported of %d total", imported, len(themes))
	}

	userPath := userAppearanceThemesPath()
	if userPath == "" {
		t.Fatal("expected user themes path")
	}
	if _, err := os.Stat(userPath); err != nil {
		t.Fatalf("expected user themes file: %v", err)
	}
}

func TestUpsertImportedThemesPreservesExistingFromOtherFile(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	resetThemeRegistryForTest()

	first := []ThemeDefinition{
		{ID: "aqua", Label: "Aqua Old", Source: "imported", Appearance: appearanceConfig{}},
		{ID: "luna", Label: "Luna", Source: "imported", Appearance: appearanceConfig{}},
	}
	if err := upsertImportedThemes(first, filepath.Join("skins", "first.yaml")); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := []ThemeDefinition{
		{ID: "aqua", Label: "Aqua New", Source: "imported", Appearance: appearanceConfig{}},
		{ID: "google", Label: "Google", Source: "imported", Appearance: appearanceConfig{}},
	}
	if err := upsertImportedThemes(second, filepath.Join("skins", "second.yaml")); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	themes, err := loadAppearanceThemesFile(userAppearanceThemesPath())
	if err != nil {
		t.Fatalf("load user themes: %v", err)
	}
	byID := make(map[string]ThemeDefinition, len(themes))
	for _, theme := range themes {
		byID[theme.ID] = theme
	}
	if len(byID) != 3 {
		t.Fatalf("expected 3 themes after merge, got %d: %#v", len(byID), byID)
	}
	if byID["aqua"].Label != "Aqua Old" {
		t.Fatalf("expected existing aqua preserved, got label %q source_file %q", byID["aqua"].Label, byID["aqua"].SourceFile)
	}
	if _, ok := byID["luna"]; !ok {
		t.Fatal("expected luna preserved")
	}
	if _, ok := byID["google"]; !ok {
		t.Fatal("expected google added")
	}
}

func TestUpsertImportedThemesRefreshesSameSourceFile(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	resetThemeRegistryForTest()

	first := []ThemeDefinition{
		{ID: "aqua", Label: "Aqua v1", Source: "imported", Appearance: appearanceConfig{}},
	}
	if err := upsertImportedThemes(first, "weasel.yaml"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := []ThemeDefinition{
		{ID: "aqua", Label: "Aqua v2", Source: "imported", Appearance: appearanceConfig{}},
	}
	if err := upsertImportedThemes(second, "weasel.yaml"); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	themes, err := loadAppearanceThemesFile(userAppearanceThemesPath())
	if err != nil {
		t.Fatalf("load user themes: %v", err)
	}
	if len(themes) != 1 {
		t.Fatalf("expected 1 theme, got %d", len(themes))
	}
	if themes[0].Label != "Aqua v2" {
		t.Fatalf("expected same-file re-import to refresh, got %q", themes[0].Label)
	}
}

func TestAppearanceMenuThemeSubmenuDynamic(t *testing.T) {
	ensureTestThemeRegistry(t)
	ime := newTestIME()
	items := ime.buildMenu()
	appearanceMenu := findMenuByText(items, "外观(&A)")
	if appearanceMenu == nil {
		t.Fatal("expected appearance menu")
	}
	submenu, ok := appearanceMenu["submenu"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected appearance submenu")
	}
	themeGroup := findMenuByText(submenu, "切换主题")
	if themeGroup == nil {
		t.Fatal("expected theme submenu group")
	}
	themeItems, ok := themeGroup["submenu"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected theme items")
	}
	if len(themeItems) < 14 {
		t.Fatalf("expected at least 14 themes in menu, got %d", len(themeItems))
	}
	foundPurple := false
	for _, item := range themeItems {
		if text, _ := item["text"].(string); strings.Contains(text, "韵味") {
			foundPurple = true
			if id, ok := item["id"].(int); !ok || id < ID_APPEARANCE_THEME_BASE {
				t.Fatalf("unexpected theme command id: %#v", item["id"])
			}
		}
	}
	if !foundPurple {
		t.Fatal("expected purple theme in menu")
	}
}
