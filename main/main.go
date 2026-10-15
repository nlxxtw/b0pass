package main

import (
	_ "b0go/apps/docs"
	"b0go/apps/panel"
	_ "b0go/apps/pass"
	"os"
	"runtime"
	"strings"
	"time"

	"b0go/core/engine"
	"b0go/core/gateway"
	"b0go/core/tools/cmd"
	"b0go/core/tools/files"
	"b0go/core/tools/nets"

	"github.com/logrusorgru/aurora"
)

func main() {
	runtime.LockOSThread()

	configFile := "config.ini"
	defaultConfig := "[gateway]\nListenAddr = \":8000\"\nDomain=\"\"\nLive = false\n\n[pass]\nPath = \"files\"\nCodeReadOnly = \"\"\nCodeReadWrite = \"admin\"\nLive = false\n"
	ok, _ := files.PathExists(configFile)
	if !ok {
		_ = os.WriteFile(configFile, []byte(defaultConfig), 0666)
		_ = os.MkdirAll("files", 0666)
	}

	// Windows：只用原生工具面板（无 WebView2、无黑框控制台）
	if runtime.GOOS == "windows" {
		if panel.RunDesktop() {
			return
		}
		// 面板失败不再静默退回黑框，避免「又变回以前那种」
		return
	}

	// 非 Windows：控制台自动启动
	gateway.AutoListen = true
	go func() {
		for i := 0; i < 80 && !gateway.IsRunning(); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		engine.Print(aurora.Black("--------------------------------------------"))
		engine.Print(aurora.BgGreen(aurora.Black("              NextPass 局域网作业助手              ")))
		ip := nets.GetOutBoundIP()
		serverUrl := ""
		if strings.TrimSpace(engine.Domain) != "" {
			serverUrl = engine.Domain
		} else {
			addr := gateway.CurrentAddr()
			ports := strings.Split(addr, ":")
			if len(ports) == 2 {
				if ports[0] != "" {
					ip = ports[0]
				}
				serverUrl = ip + ":" + ports[1]
			} else {
				serverUrl = ip + addr
			}
		}
		engine.Print(aurora.BrightBlue("端口配置为：" + gateway.CurrentAddr()))
		engine.Print(aurora.BrightBlue("学生访问： http://" + serverUrl + "/"))
		engine.Print(aurora.BrightBlue("老师管理： http://" + serverUrl + "/admin"))
		engine.Print(aurora.Green("需特别注意：本机【防火墙】设为：允许访问"))
		engine.Print(aurora.Black("--------------------------------------------"))
		_ = cmd.Open("http://" + serverUrl + "/admin")
	}()
	time.Sleep(20 * time.Millisecond)
	_ = engine.Run(configFile)
	select {}
}
