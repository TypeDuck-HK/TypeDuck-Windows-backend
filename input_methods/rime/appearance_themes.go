package rime

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

//go:embed appearance_themes.json
var embeddedBuiltinAppearanceThemes []byte

const (
	appearanceThemesFileName = "appearance_themes.json"
	themeRegistryVersionInit = 1
)

type ThemeDefinition struct {
	ID         string           `json:"id"`
	Label      string           `json:"label"`
	Source     string           `json:"source"`
	SourceFile string           `json:"source_file,omitempty"`
	Palette    ThemePalette     `json:"palette,omitempty"`
	Appearance appearanceConfig `json:"appearance"`
}

type ThemePalette map[string]string

type appearanceThemesFile struct {
	Version int                    `json:"version"`
	Fonts   map[string]interface{} `json:"fonts,omitempty"`
	Themes  []ThemeDefinition      `json:"themes"`
}

var (
	themeRegistryMu      sync.RWMutex
	themeRegistryCache   map[string]ThemeDefinition
	themeRegistryList    []ThemeDefinition
	themeRegistryVersion atomic.Uint64
)

func resetThemeRegistryForTest() {
	themeRegistryMu.Lock()
	defer themeRegistryMu.Unlock()
	themeRegistryCache = nil
	themeRegistryList = nil
	themeRegistryVersion.Store(0)
}

func bumpThemeRegistryVersion() uint64 {
	return themeRegistryVersion.Add(1)
}

func currentThemeRegistryVersion() uint64 {
	return themeRegistryVersion.Load()
}

func builtinAppearanceThemesPath() string {
	candidates := []string{}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "input_methods", "rime", appearanceThemesFileName),
			filepath.Join(exeDir, "input_methods", "rime", "data", appearanceThemesFileName),
		)
	}
	candidates = append(candidates,
		filepath.Join("input_methods", "rime", appearanceThemesFileName),
		filepath.Join("input_methods", "rime", "data", appearanceThemesFileName),
		filepath.Join("data", appearanceThemesFileName),
	)
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func loadBuiltinThemes() ([]ThemeDefinition, error) {
	if path := builtinAppearanceThemesPath(); path != "" {
		themes, err := loadAppearanceThemesFile(path)
		if err != nil {
			return nil, err
		}
		if len(themes) > 0 {
			return themes, nil
		}
	}
	if len(embeddedBuiltinAppearanceThemes) == 0 {
		return nil, nil
	}
	var file appearanceThemesFile
	if err := json.Unmarshal(embeddedBuiltinAppearanceThemes, &file); err != nil {
		return nil, err
	}
	return file.Themes, nil
}

func userAppearanceThemesPath() string {
	root := moqiAppDataDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, appearanceThemesFileName)
}

func loadAppearanceThemesFile(path string) ([]ThemeDefinition, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var file appearanceThemesFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	return file.Themes, nil
}

func mergeThemeDefinitions(builtinThemes, userThemes []ThemeDefinition) []ThemeDefinition {
	merged := make(map[string]ThemeDefinition, len(builtinThemes)+len(userThemes))
	builtinIDs := make(map[string]struct{}, len(builtinThemes))
	for _, theme := range builtinThemes {
		if strings.TrimSpace(theme.ID) == "" {
			continue
		}
		theme = normalizeThemeDefinition(theme)
		merged[theme.ID] = theme
		builtinIDs[theme.ID] = struct{}{}
	}
	for _, theme := range userThemes {
		if strings.TrimSpace(theme.ID) == "" {
			continue
		}
		theme = normalizeThemeDefinition(theme)
		if _, isBuiltin := builtinIDs[theme.ID]; isBuiltin && theme.Source == "imported" {
			// 导入主题与内置同 id 时保留内置，导入项使用独立 id 避免“覆盖”内置主题
			theme.ID = importedThemeIDForCollision(theme.ID)
			theme.Label = theme.Label + " (导入)"
		}
		if _, exists := merged[theme.ID]; exists {
			continue
		}
		merged[theme.ID] = theme
	}
	list := make([]ThemeDefinition, 0, len(merged))
	for _, theme := range merged {
		list = append(list, theme)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Label == list[j].Label {
			return list[i].ID < list[j].ID
		}
		return list[i].Label < list[j].Label
	})
	return list
}

func normalizeThemeID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func importedThemeIDForCollision(id string) string {
	id = normalizeThemeID(id)
	if strings.HasPrefix(id, "imported/") {
		return id
	}
	return "imported/" + id
}

func normalizeThemeDefinition(theme ThemeDefinition) ThemeDefinition {
	theme.ID = normalizeThemeID(theme.ID)
	theme.Label = strings.TrimSpace(theme.Label)
	if theme.Label == "" {
		theme.Label = theme.ID
	}
	theme.Source = strings.TrimSpace(theme.Source)
	if theme.Source == "" {
		theme.Source = "imported"
	}
	theme.SourceFile = strings.TrimSpace(theme.SourceFile)
	return theme
}

func reloadThemeRegistry() error {
	builtinThemes, err := loadBuiltinThemes()
	if err != nil {
		return err
	}
	userThemes, err := loadAppearanceThemesFile(userAppearanceThemesPath())
	if err != nil {
		return err
	}
	list := mergeThemeDefinitions(builtinThemes, userThemes)
	cache := make(map[string]ThemeDefinition, len(list))
	for _, theme := range list {
		cache[theme.ID] = theme
	}
	themeRegistryMu.Lock()
	themeRegistryCache = cache
	themeRegistryList = list
	themeRegistryMu.Unlock()
	bumpThemeRegistryVersion()
	return nil
}

func ensureThemeRegistryLoaded() {
	if currentThemeRegistryVersion() > 0 {
		return
	}
	_ = reloadThemeRegistry()
	if currentThemeRegistryVersion() == 0 {
		bumpThemeRegistryVersion()
	}
}

func listThemes() []ThemeDefinition {
	ensureThemeRegistryLoaded()
	themeRegistryMu.RLock()
	defer themeRegistryMu.RUnlock()
	if len(themeRegistryList) == 0 {
		return nil
	}
	cloned := make([]ThemeDefinition, len(themeRegistryList))
	copy(cloned, themeRegistryList)
	return cloned
}

func lookupRegisteredTheme(id string) (ThemeDefinition, bool) {
	ensureThemeRegistryLoaded()
	themeRegistryMu.RLock()
	defer themeRegistryMu.RUnlock()
	theme, ok := themeRegistryCache[normalizeThemeID(id)]
	return theme, ok
}

func isRegisteredTheme(theme string) bool {
	_, ok := lookupRegisteredTheme(theme)
	return ok
}

func themeAppearanceConfig(theme ThemeDefinition) appearanceConfig {
	cfg := paletteAppearanceConfig(theme.Palette)
	if len(theme.Palette) == 0 {
		cfg = cloneAppearanceConfig(theme.Appearance)
	}
	themeID := theme.ID
	cfg.CandidateTheme = &themeID
	return cfg
}

func paletteAppearanceConfig(palette ThemePalette) appearanceConfig {
	var cfg appearanceConfig
	if len(palette) == 0 {
		return cfg
	}
	setString := func(value string) *string {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		return &value
	}
	cfg.CandidateBackgroundColor = setString(palette["panel_background"])
	cfg.CandidateHighlightColor = setString(palette["selection_background"])
	cfg.CandidateTextColor = setString(palette["text_primary"])
	if cfg.CandidateTextColor == nil {
		cfg.CandidateTextColor = setString(palette["selection_text"])
	}
	cfg.CandidateHighlightTextColor = setString(palette["selection_text"])
	if cfg.CandidateHighlightTextColor == nil {
		cfg.CandidateHighlightTextColor = setString(palette["text_primary"])
	}
	cfg.CandidateCommentColor = setString(palette["definition_text"])
	if cfg.CandidateCommentColor == nil {
		cfg.CandidateCommentColor = setString(palette["text_secondary"])
	}
	cfg.CandidateCommentHighlightColor = setString(palette["selection_text"])
	if cfg.CandidateCommentHighlightColor == nil {
		cfg.CandidateCommentHighlightColor = setString(palette["text_primary"])
	}
	return cfg
}

func (ime *IME) applyThemeByID(id string) bool {
	theme, ok := lookupRegisteredTheme(id)
	if !ok {
		return false
	}
	cfg := themeAppearanceConfig(theme)
	ime.applyAppearanceConfig(cfg)
	ime.saveAppearancePrefsWithReason("applyThemeByID:" + theme.ID)
	return true
}

func upsertImportedThemes(themes []ThemeDefinition, sourceFile string) error {
	if len(themes) == 0 {
		return nil
	}
	path := userAppearanceThemesPath()
	if path == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing, err := loadAppearanceThemesFile(path)
	if err != nil {
		return err
	}
	byID := make(map[string]ThemeDefinition, len(existing)+len(themes))
	for _, theme := range existing {
		if strings.TrimSpace(theme.ID) == "" {
			continue
		}
		byID[normalizeThemeID(theme.ID)] = normalizeThemeDefinition(theme)
	}
	baseName := filepath.Base(sourceFile)
	for _, theme := range themes {
		theme = normalizeThemeDefinition(theme)
		theme.Source = "imported"
		if baseName != "" {
			theme.SourceFile = baseName
		}
		existingTheme, exists := byID[theme.ID]
		if exists {
			// 同 id 去重：仅当来自同一皮肤文件时刷新，不覆盖其它来源或已有条目
			if existingTheme.SourceFile == baseName && existingTheme.Source == "imported" {
				byID[theme.ID] = theme
			}
			continue
		}
		byID[theme.ID] = theme
	}
	merged := make([]ThemeDefinition, 0, len(byID))
	for _, theme := range byID {
		merged = append(merged, theme)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Label == merged[j].Label {
			return merged[i].ID < merged[j].ID
		}
		return merged[i].Label < merged[j].Label
	})
	file := appearanceThemesFile{Version: themeRegistryVersionInit, Themes: merged}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return reloadThemeRegistry()
}

func themeCommandID(index int) int {
	return ID_APPEARANCE_THEME_BASE + index
}

func themeCommandIndex(commandID int) (int, bool) {
	index := commandID - ID_APPEARANCE_THEME_BASE
	if index < 0 {
		return 0, false
	}
	return index, true
}

func (ime *IME) handleThemeCommand(commandID int) bool {
	index, ok := themeCommandIndex(commandID)
	if !ok {
		return false
	}
	themes := listThemes()
	if index < 0 || index >= len(themes) {
		return false
	}
	return ime.applyThemeByID(themes[index].ID)
}

func (ime *IME) themeMenuItems() []map[string]interface{} {
	themes := listThemes()
	items := make([]map[string]interface{}, 0, len(themes))
	current := strings.ToLower(strings.TrimSpace(ime.style.CandidateTheme))
	for index, theme := range themes {
		items = append(items, map[string]interface{}{
			"id":      themeCommandID(index),
			"text":    theme.Label,
			"checked": current == theme.ID,
		})
	}
	return items
}

func applyThemeAppearanceToStyle(ime *IME, theme ThemeDefinition) {
	cfg := themeAppearanceConfig(theme)
	ime.style.CandidateTheme = theme.ID
	if cfg.CandidateBackgroundColor != nil {
		ime.style.CandidateBackgroundColor = *cfg.CandidateBackgroundColor
	}
	if cfg.CandidateHighlightColor != nil {
		ime.style.CandidateHighlightColor = *cfg.CandidateHighlightColor
	}
	if cfg.CandidateTextColor != nil {
		ime.style.CandidateTextColor = *cfg.CandidateTextColor
		ime.style.CandidateCommentColor = ime.style.CandidateTextColor
	}
	if cfg.CandidateHighlightTextColor != nil {
		ime.style.CandidateHighlightTextColor = *cfg.CandidateHighlightTextColor
		ime.style.CandidateCommentHighlightColor = ime.style.CandidateHighlightTextColor
	}
	if cfg.CandidateCommentColor != nil {
		ime.style.CandidateCommentColor = *cfg.CandidateCommentColor
	}
	if cfg.CandidateCommentHighlightColor != nil {
		ime.style.CandidateCommentHighlightColor = *cfg.CandidateCommentHighlightColor
	}
	if cfg.FontFace != nil && strings.TrimSpace(*cfg.FontFace) != "" {
		ime.style.FontFace = strings.TrimSpace(*cfg.FontFace)
	}
	if cfg.FontPoint != nil && *cfg.FontPoint > 0 {
		ime.style.FontPoint = *cfg.FontPoint
	}
	if cfg.CandidateCommentFontFace != nil && strings.TrimSpace(*cfg.CandidateCommentFontFace) != "" {
		ime.style.CandidateCommentFontFace = strings.TrimSpace(*cfg.CandidateCommentFontFace)
	}
	if cfg.CandidateCommentFontPoint != nil && *cfg.CandidateCommentFontPoint > 0 {
		ime.style.CandidateCommentFontPoint = *cfg.CandidateCommentFontPoint
	}
	if cfg.InlinePreedit != nil {
		if *cfg.InlinePreedit {
			ime.style.InlinePreedit = "composition"
		} else {
			ime.style.InlinePreedit = "external"
		}
	}
	if cfg.CandidatePerRow != nil && *cfg.CandidatePerRow > 0 {
		ime.style.CandidatePerRow = *cfg.CandidatePerRow
	}
	if cfg.CandidateCount != nil && *cfg.CandidateCount > 0 {
		ime.style.CandidateCount = *cfg.CandidateCount
	}
	if cfg.CandidateSpacing != nil && *cfg.CandidateSpacing >= 0 {
		ime.style.CandidateSpacing = *cfg.CandidateSpacing
	}
}
