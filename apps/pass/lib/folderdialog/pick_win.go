//go:build windows

package folderdialog

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func readUTF8PathFile(tmp string) string {
	b, err := os.ReadFile(tmp)
	_ = os.Remove(tmp)
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(b))
	return strings.TrimPrefix(path, "\ufeff")
}

func runPS(ps string) (string, error) {
	tmp := filepath.Join(os.TempDir(), "nextpass-folder-pick.txt")
	_ = os.Remove(tmp)
	escTmp := strings.ReplaceAll(tmp, "'", "''")
	ps = strings.ReplaceAll(ps, "{{TMP}}", escTmp)
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = out
		return "", err
	}
	return readUTF8PathFile(tmp), nil
}

// Pick 弹出本机文件夹选择框，返回绝对路径（UTF-8，支持中文）。
func Pick(startDir, title string) (string, error) {
	if title == "" {
		title = "选择目录"
	}
	escTitle := strings.ReplaceAll(title, "'", "''")
	escStart := strings.ReplaceAll(startDir, "'", "''")

	ps := `
Add-Type -AssemblyName System.Windows.Forms
$utf8 = New-Object System.Text.UTF8Encoding $false
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
  [System.IO.File]::WriteAllText('{{TMP}}', $d.SelectedPath, $utf8)
}
`
	return runPS(ps)
}

// PickFile 弹出本机文件选择框，返回所选文件绝对路径。
func PickFile(startDir, title string) (string, error) {
	if title == "" {
		title = "选择文件"
	}
	escTitle := strings.ReplaceAll(title, "'", "''")
	escStart := strings.ReplaceAll(startDir, "'", "''")
	ps := `
Add-Type -AssemblyName System.Windows.Forms
$utf8 = New-Object System.Text.UTF8Encoding $false
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = '` + escTitle + `'
$d.Filter = '所有文件 (*.*)|*.*'
$d.Multiselect = $false
$d.CheckFileExists = $true
`
	if startDir != "" {
		ps += `
if (Test-Path -LiteralPath '` + escStart + `') { $d.InitialDirectory = '` + escStart + `' }
`
	}
	ps += `
$r = $d.ShowDialog()
if ($r -eq [System.Windows.Forms.DialogResult]::OK) {
  [System.IO.File]::WriteAllText('{{TMP}}', $d.FileName, $utf8)
}
`
	return runPS(ps)
}

// PickFileOrFolder 先询问选文件还是文件夹，再弹出对应对话框。
func PickFileOrFolder(startDir string) (string, error) {
	escStart := strings.ReplaceAll(startDir, "'", "''")
	ps := `
Add-Type -AssemblyName System.Windows.Forms
$utf8 = New-Object System.Text.UTF8Encoding $false
$choice = [System.Windows.Forms.MessageBox]::Show(
  "选「是」= 选择文件" + [char]10 + "选「否」= 选择文件夹",
  "NextPass",
  [System.Windows.Forms.MessageBoxButtons]::YesNoCancel,
  [System.Windows.Forms.MessageBoxIcon]::Question
)
if ($choice -eq [System.Windows.Forms.DialogResult]::Cancel) { exit 0 }
if ($choice -eq [System.Windows.Forms.DialogResult]::Yes) {
  $d = New-Object System.Windows.Forms.OpenFileDialog
  $d.Title = '选择要发送的文件（将分享其所在文件夹）'
  $d.Filter = '所有文件 (*.*)|*.*'
  $d.Multiselect = $false
  $d.CheckFileExists = $true
`
	if startDir != "" {
		ps += `
  if (Test-Path -LiteralPath '` + escStart + `') { $d.InitialDirectory = '` + escStart + `' }
`
	}
	ps += `
  $r = $d.ShowDialog()
  if ($r -eq [System.Windows.Forms.DialogResult]::OK) {
    [System.IO.File]::WriteAllText('{{TMP}}', $d.FileName, $utf8)
  }
} else {
  $d = New-Object System.Windows.Forms.FolderBrowserDialog
  $d.Description = '选择要发给学生的本地文件夹'
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
    [System.IO.File]::WriteAllText('{{TMP}}', $d.SelectedPath, $utf8)
  }
}
`
	return runPS(ps)
}
