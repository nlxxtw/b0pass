//go:build windows
// +build windows

package cmd

import (
	"log"
	"os/exec"
	"strings"
	"syscall"
)

// Open 打开浏览器（Windows）
// 注意：http(s) URL 必须保持正斜杠，不能改成反斜杠，否则带路径（如 /admin）时无法弹出浏览器。
func Open(uri string) error {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil
	}

	// 本地文件路径才需要反斜杠；网页地址保持原样
	if !(strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://")) {
		uri = strings.ReplaceAll(uri, "/", "\\")
	}

	log.Println("[CommandAs] cmd /c start", uri)
	// start 的第一个引号参数是窗口标题，必须留空，否则 URL 会被当成标题
	cmd := exec.Command("cmd", "/c", "start", "", uri)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
