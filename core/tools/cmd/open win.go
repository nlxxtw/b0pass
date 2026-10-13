//go:build windows
// +build windows

package cmd

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Open 打开 URL 或本地路径。
// 本地路径改用 explorer（支持中文/空格，比 cmd start 稳定）；网页仍用 start。
func Open(uri string) error {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil
	}
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		log.Println("[CommandAs] cmd /c start url", uri)
		cmd := exec.Command("cmd", "/c", "start", "", uri)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return cmd.Start()
	}
	return Reveal(uri)
}

// Reveal 用资源管理器打开文件夹，或选中文件。
func Reveal(path string) error {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, "/", "\\")
	st, err := os.Stat(path)
	if err != nil {
		log.Println("[CommandAs] reveal missing:", path, err)
		return err
	}
	var cmd *exec.Cmd
	if st.IsDir() {
		log.Println("[CommandAs] explorer dir", path)
		cmd = exec.Command("explorer.exe", path)
	} else {
		log.Println("[CommandAs] explorer select", path)
		cmd = exec.Command("explorer.exe", "/select,"+path)
	}
	// 不要 HideWindow，否则部分环境 explorer 会闪一下就退
	return cmd.Start()
}
