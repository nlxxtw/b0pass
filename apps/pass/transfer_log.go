package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// countingWriter 统计实际写出字节（下载测速用）；首包后立刻 Flush，浏览器更快弹出下载栏
type countingWriter struct {
	gin.ResponseWriter
	n      int64
	flushed bool
}

func (w *countingWriter) flushOnce() {
	if w.flushed {
		return
	}
	w.flushed = true
	if f, ok := w.ResponseWriter.(interface{ Flush() }); ok {
		f.Flush()
	}
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.n += int64(n)
	if n > 0 {
		w.flushOnce()
	}
	return n, err
}

func (w *countingWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	w.n += int64(n)
	if n > 0 {
		w.flushOnce()
	}
	return n, err
}

// 传输测速日志：控制台 + files/_nextpass/transfer.log
// 教室测试时看这个文件即可判断是否接近跑满带宽。

func transferLogFile() string {
	root := strings.TrimSpace(config.Path)
	if root == "" {
		root = "files"
	}
	return filepath.Join(root, "_nextpass", "transfer.log")
}

func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(u), 0
	for v := n / u; v >= u; v /= u {
		div *= u
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.2f %s", float64(n)/float64(div), units[exp])
}

func humanMbps(bytes int64, d time.Duration) float64 {
	if d <= 0 || bytes <= 0 {
		return 0
	}
	sec := d.Seconds()
	if sec <= 0 {
		return 0
	}
	return float64(bytes) * 8 / sec / 1e6
}

func transferHint(mbps float64, bytes int64, ranged bool) string {
	if ranged {
		return "分段Range请求(Safari等常见)；单次不全量"
	}
	if bytes < 8<<20 { // <8MB 测速参考价值低
		return "文件较小，测速仅供参考"
	}
	switch {
	case mbps >= 700:
		return "接近千兆满速，表现正常"
	case mbps >= 300:
		return "较快；未满千兆可能受磁盘/对端/交换机影响"
	case mbps >= 80:
		return "中等；常见于较好的 Wi-Fi 或百兆链路"
	case mbps >= 20:
		return "偏慢；多为教室 Wi-Fi 拥堵/信号弱/多人共用"
	default:
		return "很慢；优先查: 是否跨网段、杀软扫描、磁盘慢、Wi-Fi 差、浏览器限速"
	}
}

func appendTransferLog(line string) {
	log.Println(line)
	path := transferLogFile()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " " + line + "\n")
}

// logTransfer 记录一次上传/下载吞吐
func logTransfer(kind, name, clientIP string, bytes int64, d time.Duration, extra string) {
	mbps := humanMbps(bytes, d)
	ranged := strings.Contains(extra, "range=")
	hint := transferHint(mbps, bytes, ranged)
	line := fmt.Sprintf(
		"[TRANSFER] %s name=%q ip=%s size=%s (%d) cost=%s speed=%.1fMbps | %s",
		kind,
		name,
		clientIP,
		humanBytes(bytes),
		bytes,
		d.Round(time.Millisecond),
		mbps,
		hint,
	)
	if extra != "" {
		line += " | " + extra
	}
	appendTransferLog(line)
}
