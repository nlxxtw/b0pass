//go:build windows
// +build windows

package keys

// SendKey 主电脑键盘遥控（NextPass 作业场景默认关闭，避免强制依赖 CGO/robotgo）
func SendKey(k string) {
	// no-op
}
