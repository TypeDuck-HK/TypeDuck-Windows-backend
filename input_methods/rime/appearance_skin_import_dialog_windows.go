//go:build windows

package rime

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func promptAppearanceSkinFile(ctx context.Context) (string, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.OpenFileDialog
$dialog.Title = '导入皮肤'
$dialog.Filter = '小狼毫皮肤 (weasel.yaml)|weasel.yaml|皮肤文件 (*.yaml;*.yml;*.json)|*.yaml;*.yml;*.json|YAML 文件 (*.yaml;*.yml)|*.yaml;*.yml|JSON 文件 (*.json)|*.json|所有文件 (*.*)|*.*'
$dialog.FilterIndex = 1
$dialog.Multiselect = $false

function Find-WeaselDataDir {
  $candidates = @(
    'D:\Program Files\Rime\weasel-0.17.4\data',
    "$env:ProgramFiles\Rime\weasel-0.17.4\data",
    "$env:ProgramFiles(x86)\Rime\weasel-0.17.4\data"
  )
  foreach ($dir in $candidates) {
    if (Test-Path (Join-Path $dir 'weasel.yaml')) { return $dir }
  }
  foreach ($rimeRoot in @('D:\Program Files\Rime', "$env:ProgramFiles\Rime", "$env:ProgramFiles(x86)\Rime")) {
    if (-not (Test-Path $rimeRoot)) { continue }
    $match = Get-ChildItem -Path $rimeRoot -Directory -Filter 'weasel-*' -ErrorAction SilentlyContinue |
      Where-Object { Test-Path (Join-Path $_.FullName 'data\weasel.yaml') } |
      Sort-Object { $_.Name } -Descending |
      Select-Object -First 1
    if ($match) { return (Join-Path $match.FullName 'data') }
  }
  return $null
}

$initialDir = Find-WeaselDataDir
if ($initialDir) {
  $dialog.InitialDirectory = $initialDir
  $dialog.FileName = 'weasel.yaml'
}

if ($dialog.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK) {
  exit 2
}
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
Write-Output $dialog.FileName
`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-EncodedCommand", encodePowerShellCommand(script))
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			return "", errors.New("已取消导入")
		}
		if exitErr != nil {
			message := strings.TrimSpace(string(exitErr.Stderr))
			if message != "" {
				return "", fmt.Errorf("打开文件选择窗口失败: %s", message)
			}
		}
		return "", fmt.Errorf("打开文件选择窗口失败: %w", err)
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", errors.New("未选择皮肤文件")
	}
	return path, nil
}
