package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"b0go/core/engine"

	"github.com/logrusorgru/aurora"
)

// AutoListen 为 false 时，gateway.Run 只注册路由，不阻塞监听（供桌面面板启停）。
var AutoListen = true

var (
	serveMu     sync.Mutex
	httpSrv     *http.Server
	serveCancel context.CancelFunc
	serving     bool
	listenAddr  string
	startedAt   time.Time
)

// IsRunning 服务是否在听端口
func IsRunning() bool {
	serveMu.Lock()
	defer serveMu.Unlock()
	return serving
}

// CurrentAddr 当前监听地址（如 :8000）
func CurrentAddr() string {
	serveMu.Lock()
	defer serveMu.Unlock()
	if listenAddr != "" {
		return listenAddr
	}
	return strings.TrimSpace(config.ListenAddr)
}

// StartedAt 本次启动时间
func StartedAt() time.Time {
	serveMu.Lock()
	defer serveMu.Unlock()
	return startedAt
}

// StartServer 启动 HTTP（若已在跑则先停再启）。WriteTimeout=0 保证大文件下载不被掐断。
func StartServer(addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = ":8000"
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	_ = StopServer()

	ctx, cancel := context.WithCancel(context.Background())
	srv := &http.Server{
		Addr:              addr,
		Handler:           engine.Gin,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	serveMu.Lock()
	httpSrv = srv
	serveCancel = cancel
	listenAddr = addr
	engine.Addr = addr
	engine.Domain = config.Domain
	config.ListenAddr = addr
	serveMu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		engine.Print(aurora.Green("HTTP 监听: "), addr)
		errCh <- srv.ListenAndServe()
	}()

	// 短暂等待：端口占用等立即失败（不持锁）
	select {
	case err := <-errCh:
		cancel()
		serveMu.Lock()
		httpSrv = nil
		serveCancel = nil
		serving = false
		serveMu.Unlock()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return errors.New("服务未能启动")
	case <-time.After(350 * time.Millisecond):
	}

	serveMu.Lock()
	serving = true
	startedAt = time.Now()
	serveMu.Unlock()

	go watchAndRecover(ctx, addr, errCh)
	return nil
}

func watchAndRecover(ctx context.Context, addr string, errCh <-chan error) {
	probe := localProbeAddr(addr)
	fail := 0
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			serveMu.Lock()
			still := serving && httpSrv != nil
			serveMu.Unlock()
			if !still {
				return
			}
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				engine.Print(aurora.Red("HTTP 服务异常: "), err)
			}
			// 非主动 Stop：尝试恢复
			time.Sleep(1500 * time.Millisecond)
			serveMu.Lock()
			want := serving
			serveMu.Unlock()
			if want {
				engine.Print(aurora.Yellow("尝试重新监听…"))
				_ = StartServer(addr)
			}
			return
		case <-ticker.C:
			serveMu.Lock()
			ok := serving
			serveMu.Unlock()
			if !ok {
				return
			}
			if probe == "" || tcpAlive(probe) {
				fail = 0
				continue
			}
			fail++
			engine.Print(aurora.Yellow("本机探测端口无响应 ("), fail, "/", 3, ") ", probe)
			if fail < 3 {
				continue
			}
			engine.Print(aurora.Red("监听疑似僵死，正在自动恢复…"))
			_ = StopServer()
			time.Sleep(800 * time.Millisecond)
			_ = StartServer(addr)
			return
		}
	}
}

// StopServer 停止 HTTP
func StopServer() error {
	serveMu.Lock()
	srv := httpSrv
	cancel := serveCancel
	httpSrv = nil
	serveCancel = nil
	serving = false
	serveMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if srv == nil {
		return nil
	}
	ctx, c := context.WithTimeout(context.Background(), 3*time.Second)
	defer c()
	err := srv.Shutdown(ctx)
	_ = srv.Close()
	engine.Print(aurora.BrightBlue("HTTP 服务已停止"))
	return err
}

// NormalizeListenAddr 把「8000」或「:8000」规范成 :port
func NormalizeListenAddr(portOrAddr string) string {
	s := strings.TrimSpace(portOrAddr)
	if s == "" {
		return ":8000"
	}
	if strings.Contains(s, ":") {
		host, port, err := net.SplitHostPort(s)
		if err == nil {
			if host == "" || host == "0.0.0.0" {
				return ":" + port
			}
			return net.JoinHostPort(host, port)
		}
		if strings.HasPrefix(s, ":") {
			return s
		}
	}
	return ":" + strings.TrimPrefix(s, ":")
}

// PortOnly 从 ListenAddr 取出端口数字
func PortOnly(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "8000"
	}
	_, port, err := net.SplitHostPort(addr)
	if err == nil {
		return port
	}
	return strings.TrimPrefix(addr, ":")
}
