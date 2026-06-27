package rime

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gaboolic/moqi-ime/imecore"
)

var (
	gitLookPathFunc = exec.LookPath
	gitIsRepoFunc   = gitIsRepo
	gitPullFunc     = gitPull
)

type configUpdateState struct {
	mu      sync.Mutex
	running bool
}

var sharedConfigUpdateState configUpdateState

func resetConfigUpdateStateForTest() {
	sharedConfigUpdateState.mu.Lock()
	sharedConfigUpdateState.running = false
	sharedConfigUpdateState.mu.Unlock()
}

func trayNotification(message string, icon imecore.TrayNotificationIcon) *imecore.TrayNotification {
	return &imecore.TrayNotification{
		Title:   "Rime",
		Message: message,
		Icon:    icon,
	}
}

func (ime *IME) sendAsyncTrayNotification(notification *imecore.TrayNotification) {
	if notification == nil || ime.asyncResponseSender == nil {
		return
	}
	ime.asyncResponseSender(&imecore.Response{
		Success:          true,
		TrayNotification: notification,
	})
}

func gitIsRepo(dir string) (bool, error) {
	output, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return false, nil
	}
	topLevel := strings.TrimSpace(string(output))
	if topLevel == "" {
		return false, nil
	}
	return filepath.Clean(topLevel) == filepath.Clean(dir), nil
}

func gitPull(dir string) (string, error) {
	output, err := exec.Command("git", "-C", dir, "pull").CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func (ime *IME) updateConfigAsync(resp *imecore.Response) bool {
	if _, err := gitLookPathFunc("git"); err != nil {
		if resp != nil {
			resp.TrayNotification = trayNotification("Git command was not found", imecore.TrayNotificationIconError)
		}
		return false
	}

	userDir := ime.userDir()
	isRepo, err := gitIsRepoFunc(userDir)
	if err != nil || !isRepo {
		if resp != nil {
			resp.TrayNotification = trayNotification("Current scheme-set directory is not a Git repository", imecore.TrayNotificationIconError)
		}
		return false
	}

	sharedConfigUpdateState.mu.Lock()
	if sharedConfigUpdateState.running {
		sharedConfigUpdateState.mu.Unlock()
		if resp != nil {
			resp.TrayNotification = trayNotification("Configuration update is already running", imecore.TrayNotificationIconInfo)
		}
		return false
	}
	sharedConfigUpdateState.running = true
	sharedConfigUpdateState.mu.Unlock()

	if resp != nil {
		resp.TrayNotification = trayNotification("Starting configuration update...", imecore.TrayNotificationIconInfo)
	}

	go func(targetDir string) {
		defer func() {
			sharedConfigUpdateState.mu.Lock()
			sharedConfigUpdateState.running = false
			sharedConfigUpdateState.mu.Unlock()
		}()

		output, err := gitPullFunc(targetDir)
		if err != nil {
			message := "Configuration update failed"
			if output != "" {
				message = "Configuration update failed: " + summarizeGitOutput(output)
			}
			ime.sendAsyncTrayNotification(trayNotification(message, imecore.TrayNotificationIconError))
			return
		}

		message := "Configuration update succeeded"
		if output != "" {
			message = summarizeGitSuccessMessage(output)
		}
		ime.sendAsyncTrayNotification(trayNotification(message, imecore.TrayNotificationIconInfo))
	}(userDir)

	return true
}

func summarizeGitOutput(output string) string {
	lines := splitNonEmptyLines(output)
	if len(lines) == 0 {
		return "Check Git output"
	}
	return lastLineWithinLimit(lines, 48)
}

func summarizeGitSuccessMessage(output string) string {
	lines := splitNonEmptyLines(output)
	if len(lines) == 0 {
		return "Configuration update succeeded"
	}
	last := strings.ToLower(lines[len(lines)-1])
	switch {
	case strings.Contains(last, "already up to date"):
		return "Configuration is already up to date"
	case strings.Contains(last, "already up-to-date"):
		return "Configuration is already up to date"
	default:
		return "Configuration update succeeded"
	}
}

func splitNonEmptyLines(output string) []string {
	rawLines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func lastLineWithinLimit(lines []string, maxLen int) string {
	if len(lines) == 0 {
		return ""
	}
	last := lines[len(lines)-1]
	runes := []rune(last)
	if len(runes) <= maxLen {
		return last
	}
	return string(runes[:maxLen]) + "..."
}
