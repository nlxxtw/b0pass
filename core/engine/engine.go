package engine

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/logrusorgru/aurora"
)

var (
	// Version 引擎版本号
	Version = "2.0"
	// EngineInfo 引擎信息
	EngineInfo = &struct {
		Version          *string
		StartTime        time.Time //启动时间
		EnableWaitStream *bool
		RingSize         *int
	}{
		&Version,
		time.Now(),
		&config.EnableWaitStream,
		&config.RingSize,
	}
	// config 配置信息
	config = &struct {
		EnableWaitStream bool
		EnableAudio      bool
		EnableVideo      bool
		RingSize         int
		PublishTimeout   time.Duration
	}{
		true,
		true,
		true,
		10,
		time.Minute,
	}
	// ConfigRaw 配置信息的原始数据
	ConfigRaw []byte

	// Addr
	Addr string

	// Domain
	Domain string

	// Gin框架控制引擎
	Gin *gin.Engine
)

func init() {
	//Gin初始化设置
	gin.SetMode(gin.ReleaseMode)
	Gin = gin.New()
	// 轮询接口不打访问日志，避免控制台刷屏
	Gin.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{
			"/pass/public/sessions",
			"/pass/public/files",
			"/pass/public/update-check",
			"/pass/public/version",
			"/pass/session/list",
			"/pass/session/stats",
		},
	}))
	Gin.Use(gin.Recovery())
	// 下载/静态大文件禁止 gzip：Safari 会存成名为 download，且 pptx 等再压缩极慢
	gz := gzip.Gzip(gzip.DefaultCompression)
	Gin.Use(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/pass/public/download") ||
			strings.HasPrefix(p, "/pass/file-download") ||
			strings.HasPrefix(p, "/files") {
			c.Next()
			return
		}
		gz(c)
	})
	Gin.SetTrustedProxies(nil)
}

// Run启动Gin-Apps引擎
func Run(configFile string) (err error) {
	//Print(aurora.Black("--------------------------------------------"))
	Print(aurora.Green("Engine V"), Version, " ", aurora.BrightCyan(config))
	//停止脚本
	/*
		if runtime.GOOS == "windows" {
			os.WriteFile("stop.bat", []byte(fmt.Sprintf("taskkill /pid %d  -t  -f", os.Getpid())), 0777)
		} else {
			os.WriteFile("stop.sh", []byte(fmt.Sprintf("kill -9 %d", os.Getpid())), 0777)
		}
	*/

	//Config字典
	if ConfigRaw, err = os.ReadFile(configFile); err != nil {
		Print(aurora.Red("read config file error:"), err)
		return
	}
	var cg map[string]interface{}
	if _, err = toml.Decode(string(ConfigRaw), &cg); err == nil {
		// gateway.Run 内会阻塞在 Gin.Run，必须最后启动；
		// Go map 遍历顺序随机，否则 pass 路由经常注册不上 → /admin 404。
		start := func(name string, app *AppConfig) {
			if cfg, ok := cg[name]; ok {
				b, _ := json.Marshal(cfg)
				if err = json.Unmarshal(b, app.Config); err != nil {
					log.Println(err)
					return
				}
			} else if app.Config != nil {
				return
			}
			if app.Run != nil {
				app.Run()
			}
		}
		for name, app := range App {
			if app.Type&APP_HOOK != 0 {
				continue
			}
			start(name, app)
		}
		for name, app := range App {
			if app.Type&APP_HOOK == 0 {
				continue
			}
			start(name, app)
		}
	} else {
		Print(aurora.Red("decode config file error:"), err)
	}
	Print(aurora.Black("--------------------------------------------"))
	return
}
