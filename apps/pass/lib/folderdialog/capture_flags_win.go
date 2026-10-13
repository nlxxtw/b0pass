//go:build windows

package folderdialog

import (
	"os"
	"path/filepath"
)

func cancelFlagPath() string {
	return filepath.Join(os.TempDir(), "nextpass-drop-cancel.txt")
}

func windowsWriteCancel(on bool) error {
	p := cancelFlagPath()
	if !on {
		_ = os.Remove(p)
		return nil
	}
	return os.WriteFile(p, []byte("1"), 0644)
}
