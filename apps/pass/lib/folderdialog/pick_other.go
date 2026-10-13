//go:build !windows

package folderdialog

import (
	"errors"
	"time"
)

func Pick(startDir, title string) (string, error) {
	return "", errors.New("当前系统不支持文件夹选择框，请手动填写目录名")
}

func PickFile(startDir, title string) (string, error) {
	return "", errors.New("当前系统不支持文件选择框，请手动填写路径")
}

func PickFileOrFolder(startDir string) (string, error) {
	return "", errors.New("当前系统不支持选择框，请手动填写路径")
}

func CaptureDrop(timeout time.Duration, rect DropRect) (string, error) {
	return "", errors.New("当前系统不支持拖放识别路径")
}

func CancelCaptureDrop() {}

func ClearCaptureDropFlags() {}
