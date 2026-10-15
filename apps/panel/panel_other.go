//go:build !windows

package panel

// RunDesktop 非 Windows 无桌面面板，返回 false 走原控制台启动。
func RunDesktop() bool {
	return false
}
