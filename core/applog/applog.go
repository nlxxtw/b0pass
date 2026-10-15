package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu   sync.Mutex
	file *os.File
	path string
)

// Setup 将标准 log 同时写入文件（桌面面板「运行日志」读取）
func Setup(logPath string) error {
	mu.Lock()
	defer mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if file != nil {
		_ = file.Close()
	}
	file = f
	path = logPath
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.LstdFlags)
	return nil
}

// Path 当前日志文件路径
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return path
}

// Tail 读取日志末尾（最多 maxBytes）
func Tail(maxBytes int64) string {
	mu.Lock()
	p := path
	mu.Unlock()
	if p == "" {
		return ""
	}
	st, err := os.Stat(p)
	if err != nil {
		return ""
	}
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	size := st.Size()
	var start int64
	if size > maxBytes {
		start = size - maxBytes
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	// 若从中间截断，丢掉第一行半截
	s := string(b)
	if start > 0 {
		if i := indexByte(s, '\n'); i >= 0 && i+1 < len(s) {
			s = s[i+1:]
		}
	}
	return s
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// Info 写一行到日志
func Info(format string, args ...interface{}) {
	log.Println(fmt.Sprintf(format, args...))
}
