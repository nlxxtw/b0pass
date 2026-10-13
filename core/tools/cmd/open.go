//go:build !windows
// +build !windows

package cmd

import (
	"log"
	"os/exec"
	"runtime"
	"strings"
)

var commands = map[string]string{
	"darwin": "open",
	"linux":  "xdg-open",
}

// Open 打开浏览器或本地路径
func Open(uri string) error {
	return Reveal(uri)
}

// Reveal 打开本地路径（非 Windows）
func Reveal(uri string) error {
	uri = strings.TrimSpace(uri)
	run, ok := commands[runtime.GOOS]
	if !ok {
		log.Printf("don't know how to open things on %s platform", runtime.GOOS)
		return nil
	}
	uri = strings.ReplaceAll(uri, "\\", "/")
	cmd := exec.Command(run, uri)
	log.Println("[CommandAs]", run, uri)
	return cmd.Start()
}
