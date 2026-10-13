//go:build !windows

package folderdialog

import "errors"

func Pick(startDir, title string) (string, error) {
	return "", errors.New("当前系统不支持文件夹选择框，请手动填写目录名")
}
