package panel

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	passapp "b0go/apps/pass"
	"b0go/core/applog"
	"b0go/core/engine"
	"b0go/core/gateway"
	"b0go/core/tools/cmd"
	"b0go/core/tools/nets"

	"github.com/BurntSushi/toml"
)

var (
	bootOnce sync.Once
	bootErr  error
)

func ensureBooted() error {
	bootOnce.Do(func() {
		gateway.AutoListen = false
		logPath := filepath.Join("files", "_nextpass", "app.log")
		// 若配置里 Path 已存在，日志跟着放
		if raw, err := os.ReadFile(configFileName); err == nil {
			var ini iniFile
			if _, e := toml.Decode(string(raw), &ini); e == nil && ini.Pass.Path != "" {
				logPath = filepath.Join(ini.Pass.Path, "_nextpass", "app.log")
			}
		}
		_ = applog.Setup(logPath)
		bootErr = engine.Run(configFileName)
	})
	return bootErr
}

// State 面板状态
type State struct {
	Running   bool   `json:"running"`
	Port      string `json:"port"`
	Path      string `json:"path"`
	Password  string `json:"password"`
	AutoOpen  bool   `json:"autoOpen"`
	IP        string `json:"ip"`
	StudentURL string `json:"studentUrl"`
	AdminURL  string `json:"adminUrl"`
	Version   string `json:"version"`
	StartedAt string `json:"startedAt"`
	Message   string `json:"message"`
}

func buildState(msg string) State {
	cfg := loadPanelConfig()
	ip := nets.GetOutBoundIP()
	port := cfg.Port
	if gateway.IsRunning() {
		port = gateway.PortOnly(gateway.CurrentAddr())
	}
	base := fmt.Sprintf("http://%s:%s", ip, port)
	st := State{
		Running:    gateway.IsRunning(),
		Port:       port,
		Path:       cfg.Path,
		Password:   cfg.Password,
		AutoOpen:   cfg.AutoOpen,
		IP:         ip,
		StudentURL: base + "/",
		AdminURL:   base + "/admin",
		Version:    passapp.AppVersion,
		Message:    msg,
	}
	if t := gateway.StartedAt(); !t.IsZero() && gateway.IsRunning() {
		st.StartedAt = t.Format("15:04:05")
	}
	return st
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "only one usage") ||
		strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "eaddrinuse") ||
		(strings.Contains(msg, "bind") && strings.Contains(msg, "10048"))
}

func findFreeTCPPort() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "", err
	}
	return port, nil
}

func friendlyStartErr(err error) string {
	if isAddrInUse(err) {
		return "这个端口正被其它程序占用。请换一个端口，或关掉占用它的程序后再试。"
	}
	return "启动失败，请稍后重试。若一直不行，可换个端口再试。"
}

func doStart(openBrowser bool) (State, error) {
	if err := ensureBooted(); err != nil {
		return buildState(err.Error()), err
	}
	cfg := loadPanelConfig()
	wanted := strings.TrimSpace(cfg.Port)
	if wanted == "" {
		wanted = "8000"
	}
	addr := gateway.NormalizeListenAddr(wanted)
	err := gateway.StartServer(addr)
	note := ""
	if err != nil && isAddrInUse(err) {
		free, ferr := findFreeTCPPort()
		if ferr == nil && free != "" && free != wanted {
			cfg.Port = free
			_ = savePanelConfig(cfg)
			applyRuntimeConfig(cfg)
			err = gateway.StartServer(gateway.NormalizeListenAddr(free))
			if err == nil {
				note = fmt.Sprintf("端口 %s 已被占用，已自动改用 %s。请让学生用新地址访问。", wanted, free)
				applog.Info(note)
			}
		}
	}
	if err != nil {
		msg := friendlyStartErr(err)
		applog.Info("%s 原始错误: %v", msg, err)
		return buildState(msg), err
	}
	st := buildState("服务已启动")
	if note != "" {
		st = buildState(note)
	}
	applog.Info("服务已启动 %s 学生=%s", gateway.CurrentAddr(), st.StudentURL)
	if openBrowser || cfg.AutoOpen {
		_ = cmd.Open(st.AdminURL)
	}
	return st, nil
}

func doStop() State {
	_ = gateway.StopServer()
	applog.Info("服务已停止")
	return buildState("服务已停止")
}

func applyRuntimeConfig(c PanelConfig) {
	// 热更新内存中的 pass 配置，避免只改 ini 不生效
	if a, ok := engine.App["pass"]; ok && a != nil && a.Config != nil {
		if pc, ok := a.Config.(*passapp.AppConfig); ok && pc != nil {
			if p := strings.TrimSpace(c.Path); p != "" {
				pc.Path = p
			}
			pc.CodeReadWrite = strings.TrimSpace(c.Password)
		}
	}
}

func doSave(c PanelConfig) (State, error) {
	was := gateway.IsRunning()
	if err := savePanelConfig(c); err != nil {
		return buildState(err.Error()), err
	}
	applyRuntimeConfig(c)
	applog.Info("已保存设置 port=%s path=%s", c.Port, c.Path)
	if was {
		// 改配置后自动重启监听（端口等）
		_ = gateway.StopServer()
		time.Sleep(200 * time.Millisecond)
		return doStart(false)
	}
	return buildState("设置已保存"), nil
}

func doRestart() (State, error) {
	_ = gateway.StopServer()
	time.Sleep(200 * time.Millisecond)
	return doStart(false)
}
