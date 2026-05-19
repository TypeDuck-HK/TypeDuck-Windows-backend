package cloudclipboard

const (
	DefaultSettingsRoot = "/moqi-input-method/"
	ClipDir             = "clip"
	DictDir             = "dict"
)

func ClipPathUnder(settingsRoot string) string {
	return NormalizeDir(settingsRoot) + ClipDir + "/"
}

func DictPathUnder(settingsRoot string) string {
	return NormalizeDir(settingsRoot) + DictDir + "/"
}

func NormalizeDir(raw string) string {
	path := trimSpace(raw)
	if path == "" {
		path = DefaultSettingsRoot
	}
	if path[0] != '/' {
		path = "/" + path
	}
	if path[len(path)-1] != '/' {
		path += "/"
	}
	return path
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
