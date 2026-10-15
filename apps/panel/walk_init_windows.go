//go:build windows

package panel

import (
	"unsafe"

	"github.com/lxn/win"
)

func init() {
	// 避免 walk 创建窗口时 TTM_ADDTOOL failed
	var icc win.INITCOMMONCONTROLSEX
	icc.DwSize = uint32(unsafe.Sizeof(icc))
	icc.DwICC = win.ICC_BAR_CLASSES | win.ICC_TAB_CLASSES | win.ICC_STANDARD_CLASSES | win.ICC_WIN95_CLASSES
	win.InitCommonControlsEx(&icc)
}
