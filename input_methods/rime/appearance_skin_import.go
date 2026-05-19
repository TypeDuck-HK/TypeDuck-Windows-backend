package rime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func parseAppearanceSkinFile(path string) (appearanceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return appearanceConfig{}, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return parseMoqiAppearanceJSON(data)
	case ".yaml", ".yml":
		return parseWeaselAppearanceYAML(data)
	default:
		if json.Valid(data) {
			if cfg, err := parseMoqiAppearanceJSON(data); err == nil {
				return cfg, nil
			}
		}
		return parseWeaselAppearanceYAML(data)
	}
}

func parseMoqiAppearanceJSON(data []byte) (appearanceConfig, error) {
	var cfg appearanceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return appearanceConfig{}, fmt.Errorf("无法解析外观 JSON: %w", err)
	}
	if appearanceConfigEmpty(cfg) {
		return appearanceConfig{}, fmt.Errorf("外观 JSON 中没有可导入的字段")
	}
	return cfg, nil
}

func appearanceConfigEmpty(cfg appearanceConfig) bool {
	return cfg.CandidateTheme == nil &&
		cfg.FontFace == nil &&
		cfg.FontPoint == nil &&
		cfg.CandidateCommentFontFace == nil &&
		cfg.CandidateCommentFontPoint == nil &&
		cfg.InlinePreedit == nil &&
		cfg.CandidatePerRow == nil &&
		cfg.CandidateCount == nil &&
		cfg.CandidateSpacing == nil &&
		cfg.CandidateBackgroundColor == nil &&
		cfg.CandidateHighlightColor == nil &&
		cfg.CandidateTextColor == nil &&
		cfg.CandidateHighlightTextColor == nil &&
		cfg.CandidateCommentColor == nil &&
		cfg.CandidateCommentHighlightColor == nil
}

func parseWeaselYAMLDocument(data []byte) (map[string]interface{}, map[string]interface{}, string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, "", fmt.Errorf("无法解析皮肤 YAML: %w", err)
	}
	doc := unwrapYAMLDocument(root)
	style := mergeYAMLMaps(
		mapFromYAMLPath(doc, "style"),
		patchStyleFromYAML(doc),
	)
	schemes := mergeYAMLMaps(
		mapFromYAMLPath(doc, "preset_color_schemes"),
		patchPresetColorSchemesFromYAML(doc),
	)
	schemeName := stringValue(style["color_scheme"])
	if schemeName == "" {
		schemeName = stringValue(style["color_scheme_dark"])
	}
	if schemeName == "" && len(schemes) == 1 {
		for name := range schemes {
			schemeName = name
			break
		}
	}
	return style, schemes, schemeName, nil
}

func parseWeaselAppearanceYAML(data []byte) (appearanceConfig, error) {
	style, schemes, activeSchemeID, err := parseWeaselYAMLDocument(data)
	if err != nil {
		return appearanceConfig{}, err
	}
	activeScheme := map[string]interface{}{}
	if activeSchemeID != "" {
		if scheme, ok := schemes[activeSchemeID].(map[string]interface{}); ok {
			activeScheme = scheme
		}
	}
	if len(activeScheme) == 0 && len(schemes) == 1 {
		for _, value := range schemes {
			if scheme, ok := value.(map[string]interface{}); ok {
				activeScheme = scheme
				break
			}
		}
	}
	return weaselStyleToAppearanceConfig(style, activeScheme)
}

func parseWeaselSkinFile(path string) (string, []ThemeDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	return parseWeaselSkinFromBytes(data)
}

func parseWeaselSkinFromBytes(data []byte) (string, []ThemeDefinition, error) {
	style, schemes, activeSchemeID, err := parseWeaselYAMLDocument(data)
	if err != nil {
		return "", nil, err
	}
	if len(schemes) == 0 {
		return "", nil, fmt.Errorf("皮肤 YAML 中没有 preset_color_schemes")
	}
	themes := make([]ThemeDefinition, 0, len(schemes))
	for schemeName, value := range schemes {
		scheme, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		cfg, err := weaselStyleToAppearanceConfig(style, scheme)
		if err != nil {
			continue
		}
		id := normalizeThemeID(schemeName)
		cfg.CandidateTheme = &id
		label := firstString(scheme, "name")
		if label == "" {
			label = schemeName
		}
		themes = append(themes, ThemeDefinition{
			ID:         id,
			Label:      label,
			Source:     "imported",
			Appearance: cfg,
		})
	}
	if len(themes) == 0 {
		return "", nil, fmt.Errorf("皮肤文件中没有可导入的配色方案")
	}
	if activeSchemeID == "" {
		activeSchemeID = themes[0].ID
	} else {
		activeSchemeID = normalizeThemeID(activeSchemeID)
	}
	return activeSchemeID, themes, nil
}

type weaselColorFormat string

const (
	weaselColorFormatABGR weaselColorFormat = "abgr"
	weaselColorFormatARGB weaselColorFormat = "argb"
	weaselColorFormatRGBA weaselColorFormat = "rgba"
)

func parseWeaselColorFormat(value string) weaselColorFormat {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "argb":
		return weaselColorFormatARGB
	case "rgba":
		return weaselColorFormatRGBA
	default:
		// 小狼毫默认 abgr（与 Windows COLORREF 0x00BBGGRR 一致）
		return weaselColorFormatABGR
	}
}

func unwrapYAMLDocument(root yaml.Node) *yaml.Node {
	if len(root.Content) == 0 {
		return &root
	}
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return root.Content[0]
	}
	return &root
}

func mapFromYAMLPath(root *yaml.Node, path ...string) map[string]interface{} {
	node := root
	for _, segment := range path {
		if node == nil || node.Kind != yaml.MappingNode {
			return nil
		}
		node = yamlMappingChild(node, segment)
	}
	return yamlNodeToMap(node)
}

func yamlMappingChild(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func yamlNodeToMap(node *yaml.Node) map[string]interface{} {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.MappingNode:
		result := make(map[string]interface{}, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			result[node.Content[i].Value] = yamlNodeToInterface(node.Content[i+1])
		}
		return result
	default:
		var value interface{}
		if err := node.Decode(&value); err != nil {
			return nil
		}
		if mapped, ok := value.(map[string]interface{}); ok {
			return mapped
		}
		return nil
	}
}

func yamlNodeToInterface(node *yaml.Node) interface{} {
	if node == nil {
		return nil
	}
	var value interface{}
	if err := node.Decode(&value); err != nil {
		return nil
	}
	return value
}

func patchStyleFromYAML(root *yaml.Node) map[string]interface{} {
	patch := mapFromYAMLPath(root, "patch")
	if len(patch) == 0 {
		return nil
	}
	style := map[string]interface{}{}
	if nested, ok := patch["style"].(map[string]interface{}); ok {
		for key, value := range nested {
			style[key] = value
		}
	}
	for key, value := range patch {
		if strings.HasPrefix(key, "style/") {
			style[strings.TrimPrefix(key, "style/")] = value
		}
	}
	return style
}

func patchPresetColorSchemesFromYAML(root *yaml.Node) map[string]interface{} {
	patch := mapFromYAMLPath(root, "patch")
	if len(patch) == 0 {
		return nil
	}
	schemes := map[string]interface{}{}
	if nested, ok := patch["preset_color_schemes"].(map[string]interface{}); ok {
		for key, value := range nested {
			schemes[key] = value
		}
	}
	for key, value := range patch {
		if !strings.HasPrefix(key, "preset_color_schemes/") {
			continue
		}
		remainder := strings.TrimPrefix(key, "preset_color_schemes/")
		if scheme, ok := value.(map[string]interface{}); ok && !strings.Contains(remainder, "/") {
			schemes[remainder] = scheme
			continue
		}
		parts := strings.SplitN(remainder, "/", 2)
		if len(parts) != 2 {
			continue
		}
		schemeName, field := parts[0], parts[1]
		scheme, ok := schemes[schemeName].(map[string]interface{})
		if !ok {
			scheme = map[string]interface{}{}
			schemes[schemeName] = scheme
		}
		scheme[field] = value
	}
	return schemes
}

func mergeYAMLMaps(parts ...map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for _, part := range parts {
		for key, value := range part {
			result[key] = value
		}
	}
	return result
}

func weaselStyleToAppearanceConfig(style, scheme map[string]interface{}) (appearanceConfig, error) {
	layout, _ := style["layout"].(map[string]interface{})
	colorFormat := parseWeaselColorFormat(firstString(scheme, "color_format"))
	if colorFormat == weaselColorFormatABGR {
		if override := firstString(style, "color_format"); override != "" {
			colorFormat = parseWeaselColorFormat(override)
		}
	}
	cfg := appearanceConfig{}

	theme := "custom"
	cfg.CandidateTheme = &theme

	if value := firstString(style, "font_face"); value != "" {
		cfg.FontFace = &value
	}
	if value := intValue(style["font_point"]); value > 0 {
		cfg.FontPoint = &value
	}
	if value := firstString(style, "comment_font_face"); value != "" {
		cfg.CandidateCommentFontFace = &value
	}
	if value := intValue(style["comment_font_point"]); value > 0 {
		cfg.CandidateCommentFontPoint = &value
	}
	if value, ok := boolValue(style["inline_preedit"]); ok {
		cfg.InlinePreedit = &value
	}
	if value, ok := boolValue(style["horizontal"]); ok {
		perRow := 1
		if value {
			perRow = 9
		}
		cfg.CandidatePerRow = &perRow
	}
	if value := intValue(firstValue(style["candidate_spacing"], layout["candidate_spacing"], style["spacing"], layout["spacing"])); value >= 0 {
		cfg.CandidateSpacing = &value
	}

	if background := weaselColorToHex(firstValue(scheme["back_color"], scheme["candidate_back_color"]), colorFormat); background != "" {
		cfg.CandidateBackgroundColor = &background
	}
	if highlight := weaselColorToHex(firstValue(scheme["hilited_candidate_back_color"], scheme["hilited_back_color"]), colorFormat); highlight != "" {
		cfg.CandidateHighlightColor = &highlight
	}
	if textColor := weaselColorToHex(firstValue(scheme["candidate_text_color"], scheme["text_color"]), colorFormat); textColor != "" {
		cfg.CandidateTextColor = &textColor
	}
	if highlightText := weaselColorToHex(firstValue(scheme["hilited_candidate_text_color"], scheme["hilited_text_color"]), colorFormat); highlightText != "" {
		cfg.CandidateHighlightTextColor = &highlightText
	}
	if commentColor := weaselColorToHex(firstValue(scheme["comment_text_color"], scheme["candidate_text_color"], scheme["text_color"]), colorFormat); commentColor != "" {
		cfg.CandidateCommentColor = &commentColor
	}
	if commentHighlight := weaselColorToHex(firstValue(scheme["hilited_comment_text_color"], scheme["hilited_candidate_text_color"], scheme["hilited_text_color"]), colorFormat); commentHighlight != "" {
		cfg.CandidateCommentHighlightColor = &commentHighlight
	}

	if appearanceConfigEmpty(cfg) {
		return appearanceConfig{}, fmt.Errorf("皮肤文件中没有可导入的外观字段")
	}
	return cfg, nil
}

func firstString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringValue(values[key])); value != "" {
			return value
		}
	}
	return ""
}

func firstValue(values ...interface{}) interface{} {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func intValue(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case uint64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		typed = strings.TrimSpace(typed)
		if strings.HasPrefix(strings.ToLower(typed), "0x") {
			parsed, err := strconv.ParseInt(typed[2:], 16, 64)
			if err == nil {
				return int(parsed)
			}
		}
		parsed, err := strconv.Atoi(typed)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func boolValue(value interface{}) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "yes", "1":
			return true, true
		case "false", "no", "0":
			return false, true
		}
	}
	return false, false
}

func weaselColorToHex(value interface{}, format weaselColorFormat) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		normalized := normalizeColor(strings.TrimSpace(typed))
		if normalized != "" {
			return normalized
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(typed)), "0x") {
			return weaselIntColorToHex(int64(intValue(typed)), format)
		}
		return ""
	default:
		return weaselIntColorToHex(int64(intValue(value)), format)
	}
}

func weaselIntColorToHex(value int64, format weaselColorFormat) string {
	if value < 0 {
		value &= 0xffffffff
	}
	// 全透明色（阴影等）跳过
	if value&0xffffff == 0 && value > 0xffffff {
		return ""
	}
	if value == 0 {
		return "#000000"
	}

	var r, g, b int
	switch format {
	case weaselColorFormatARGB:
		r = int((value >> 16) & 0xff)
		g = int((value >> 8) & 0xff)
		b = int(value & 0xff)
	case weaselColorFormatRGBA:
		r = int((value >> 24) & 0xff)
		g = int((value >> 16) & 0xff)
		b = int((value >> 8) & 0xff)
	default:
		// abgr / bgr: 0xBBGGRR
		r = int(value & 0xff)
		g = int((value >> 8) & 0xff)
		b = int((value >> 16) & 0xff)
	}
	return normalizeColor(fmt.Sprintf("#%02x%02x%02x", r, g, b))
}

func (ime *IME) importAppearanceSkinFromFile(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".yaml" || ext == ".yml" {
		activeSchemeID, themes, err := parseWeaselSkinFile(path)
		if err != nil {
			return err
		}
		if err := upsertImportedThemes(themes, path); err != nil {
			return err
		}
		if !ime.applyThemeByID(activeSchemeID) {
			return fmt.Errorf("无法应用配色方案 %q", activeSchemeID)
		}
		return nil
	}

	cfg, err := parseAppearanceSkinFile(path)
	if err != nil {
		return err
	}
	ime.applyAppearanceConfig(cfg)
	ime.saveAppearancePrefsWithReason("importAppearanceSkin:" + filepath.Base(path))
	return nil
}
