package panel

import (
	"fmt"
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

func doStart(openBrowser bool) (State, error) {
	if err := ensureBooted(); err != nil {
		return buildState(err.Error()), err
	}
	cfg := loadPanelConfig()
	addr := gateway.NormalizeListenAddr(cfg.Port)
	if err := gateway.StartServer(addr); err != nil {
		applog.Info("启动失败: %v", err)
		return buildState("启动失败: " + err.Error()), err
	}
	applog.Info("服务已启动 %s 学生=%s", addr, buildState("").StudentURL)
	if openBrowser || cfg.AutoOpen {
		_ = cmd.Open(buildState("").AdminURL)
	}
	return buildState("服务已启动"), nil
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
