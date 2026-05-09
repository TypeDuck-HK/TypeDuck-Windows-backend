//go:build windows

package rime

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf16"
)

func promptSchemeSetDownloadPackage(ctx context.Context) (schemeSetDownloadPackage, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$form = New-Object System.Windows.Forms.Form
$form.Text = '下载方案集'
$form.StartPosition = 'CenterScreen'
$form.Size = New-Object System.Drawing.Size(580, 290)
$form.FormBorderStyle = [System.Windows.Forms.FormBorderStyle]::FixedDialog
$form.MaximizeBox = $false
$form.MinimizeBox = $false

$presets = @(
  @{ label = '白霜拼音 <https://github.com/gaboolic/rime-frost>'; name = '白霜拼音'; url = 'https://github.com/gaboolic/rime-frost' },
  @{ label = '墨奇音形 <https://github.com/gaboolic/rime-shuangpin-fuzhuma>'; name = '墨奇音形'; url = 'https://github.com/gaboolic/rime-shuangpin-fuzhuma' },
  @{ label = '墨奇五笔整句 <https://github.com/gaboolic/rime-wubi-sentence>'; name = '墨奇五笔整句'; url = 'https://github.com/gaboolic/rime-wubi-sentence' },
  @{ label = '自定义方案集'; name = ''; url = '' }
)

$presetLabel = New-Object System.Windows.Forms.Label
$presetLabel.Text = '预设方案集'
$presetLabel.Location = New-Object System.Drawing.Point(16, 18)
$presetLabel.Size = New-Object System.Drawing.Size(520, 22)
$form.Controls.Add($presetLabel)

$presetBox = New-Object System.Windows.Forms.ComboBox
$presetBox.Location = New-Object System.Drawing.Point(16, 42)
$presetBox.Size = New-Object System.Drawing.Size(530, 24)
$presetBox.DropDownStyle = [System.Windows.Forms.ComboBoxStyle]::DropDownList
foreach ($preset in $presets) {
  [void]$presetBox.Items.Add($preset.label)
}
$form.Controls.Add($presetBox)

$nameLabel = New-Object System.Windows.Forms.Label
$nameLabel.Text = '方案集名称（可留空自动推断）'
$nameLabel.Location = New-Object System.Drawing.Point(16, 78)
$nameLabel.Size = New-Object System.Drawing.Size(520, 22)
$form.Controls.Add($nameLabel)

$nameBox = New-Object System.Windows.Forms.TextBox
$nameBox.Location = New-Object System.Drawing.Point(16, 102)
$nameBox.Size = New-Object System.Drawing.Size(530, 24)
$form.Controls.Add($nameBox)

$urlLabel = New-Object System.Windows.Forms.Label
$urlLabel.Text = '方案集 URL（GitHub 仓库或 ZIP 下载地址）'
$urlLabel.Location = New-Object System.Drawing.Point(16, 138)
$urlLabel.Size = New-Object System.Drawing.Size(520, 22)
$form.Controls.Add($urlLabel)

$urlBox = New-Object System.Windows.Forms.TextBox
$urlBox.Location = New-Object System.Drawing.Point(16, 162)
$urlBox.Size = New-Object System.Drawing.Size(530, 24)
$form.Controls.Add($urlBox)

$okButton = New-Object System.Windows.Forms.Button
$okButton.Text = '下载'
$okButton.Location = New-Object System.Drawing.Point(370, 206)
$okButton.Size = New-Object System.Drawing.Size(80, 30)
$okButton.DialogResult = [System.Windows.Forms.DialogResult]::OK
$form.AcceptButton = $okButton
$form.Controls.Add($okButton)

$cancelButton = New-Object System.Windows.Forms.Button
$cancelButton.Text = '取消'
$cancelButton.Location = New-Object System.Drawing.Point(466, 206)
$cancelButton.Size = New-Object System.Drawing.Size(80, 30)
$cancelButton.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$form.CancelButton = $cancelButton
$form.Controls.Add($cancelButton)

$presetBox.Add_SelectedIndexChanged({
  $selected = $presets[$presetBox.SelectedIndex]
  $nameBox.Text = $selected.name
  $urlBox.Text = $selected.url
})
$presetBox.SelectedIndex = 0

$form.Add_Shown({ $presetBox.Focus() })
$result = $form.ShowDialog()
if ($result -ne [System.Windows.Forms.DialogResult]::OK) {
  exit 2
}

$payload = @{
  name = $nameBox.Text
  url = $urlBox.Text
} | ConvertTo-Json -Compress
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
Write-Output $payload
`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-EncodedCommand", encodePowerShellCommand(script))
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			return schemeSetDownloadPackage{}, errors.New("已取消下载")
		}
		if exitErr != nil {
			message := strings.TrimSpace(string(exitErr.Stderr))
			if message != "" {
				return schemeSetDownloadPackage{}, fmt.Errorf("打开下载窗口失败: %s", message)
			}
		}
		return schemeSetDownloadPackage{}, fmt.Errorf("打开下载窗口失败: %w", err)
	}

	var payload struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	jsonOutput, err := extractPowerShellJSONOutput(output)
	if err != nil {
		return schemeSetDownloadPackage{}, err
	}
	if err := json.Unmarshal(jsonOutput, &payload); err != nil {
		return schemeSetDownloadPackage{}, fmt.Errorf("读取下载窗口输入失败: %w", err)
	}
	rawURL := strings.TrimSpace(payload.URL)
	if rawURL == "" {
		return schemeSetDownloadPackage{}, errors.New("请输入方案集 ZIP 下载 URL")
	}
	return schemeSetDownloadPackage{
		Name: strings.TrimSpace(payload.Name),
		URL:  rawURL,
	}, nil
}

func extractPowerShellJSONOutput(output []byte) ([]byte, error) {
	text := strings.TrimSpace(string(output))
	start := strings.LastIndex(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("读取下载窗口输入失败: 输出为空")
	}
	return []byte(text[start : end+1]), nil
}

func encodePowerShellCommand(script string) string {
	encoded := utf16.Encode([]rune(script))
	buf := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		binary.LittleEndian.PutUint16(buf[i*2:], value)
	}
	return base64.StdEncoding.EncodeToString(buf)
}
