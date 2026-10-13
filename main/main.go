package main

import (
	_ "b0go/apps/docs"
	_ "b0go/apps/pass"
	"os"
	"strings"
	"time"

	"b0go/core/engine"
	_ "b0go/core/gateway"
	"b0go/core/tools/cmd"
	"b0go/core/tools/files"
	"b0go/core/tools/nets"

	"github.com/logrusorgru/aurora"
)

func main() {
	/**
	* 检查配置
	 */
	configFile := "config.ini"
	defaultConfig := "[gateway]\nListenAddr = \":8000\"\nDomain=\"\"\nLive = false\n\n[pass]\nPath = \"files\"\nCodeReadWrite = \"admin\"\nLive = false\n"
	ok, _ := files.PathExists(configFile)
	if !ok {
		os.WriteFile(configFile, []byte(defaultConfig), 0666)
		os.MkdirAll("files", 0666)
	}
	/**
	* 启动服务
	 */
	go func() {
		//Version
		engine.Print(aurora.Black("--------------------------------------------"))
		engine.Print(aurora.BgGreen(aurora.Black("              NextPass 局域网作业助手              ")))
		waitTime := 100000 * time.Microsecond
		time.Sleep(waitTime)
		//检查ListenAddr
		ip := nets.GetOutBoundIP()
		serverUrl := ""
		if strings.Trim(engine.Domain, "") != "" {
			serverUrl = engine.Domain
		} else {
			ports := strings.Split(engine.Addr, ":")
			if len(ports) != 2 {
				time.Sleep(waitTime)
			}
			if ports[0] != "" {
				ip = ports[0]
			}
			serverUrl = ip + ":" + ports[1]
		}

		engine.Print(aurora.BrightBlue("端口配置为：" + engine.Addr))
		engine.Print(aurora.BrightBlue("学生访问： http://" + serverUrl + "/"))
		engine.Print(aurora.BrightBlue("老师管理： http://" + serverUrl + "/admin"))
		engine.Print(aurora.Green("需特别注意：本机【防火墙】设为：允许访问"))
		engine.Print(aurora.Black("--------------------------------------------"))
		cmd.Open("http://" + serverUrl + "/admin")
	}()
	time.Sleep(5000 * time.Microsecond)
	engine.Run(configFile)
	select {}
}
