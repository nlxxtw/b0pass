//go:build windows

package folderdialog

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Pick 弹出本机文件夹选择框，返回绝对路径（UTF-8，支持中文）。
func Pick(startDir, title string) (string, error) {
	if title == "" {
		title = "选择目录"
	}
	tmp := filepath.Join(os.TempDir(), "nextpass-folder-pick.txt")
	_ = os.Remove(tmp)

	// 路径写进 UTF-8 文件，避免 PowerShell 控制台 GBK 乱码
	escTmp := strings.ReplaceAll(tmp, "'", "''")
	escTitle := strings.ReplaceAll(title, "'", "''")
	escStart := strings.ReplaceAll(startDir, "'", "''")

	ps := `
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = '` + escTitle + `'
$d.ShowNewFolderButton = $true
`
	if startDir != "" {
		ps += `
if (Test-Path -LiteralPath '` + escStart + `') { $d.SelectedPath = '` + escStart + `' }
`
	}
	ps += `
$r = $d.ShowDialog()
if ($r -eq [System.Windows.Forms.DialogResult]::OK) {
  $utf8 = New-Object System.Text.UTF8Encoding $false
  [System.IO.File]::WriteAllText('` + escTmp + `', $d.SelectedPath, $utf8)
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = out
		return "", err
	}

	b, err := os.ReadFile(tmp)
	_ = os.Remove(tmp)
	if err != nil {
		return "", nil // 用户取消
	}
	path := strings.TrimSpace(string(b))
	// 去掉可能的 UTF-8 BOM
	path = strings.TrimPrefix(path, "\ufeff")
	return path, nil
}
