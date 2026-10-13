package app

import (
	"b0go/core/engine"
	"embed"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/logrusorgru/aurora"
)

// APP:AppConfig
type AppConfig struct {
	Live          bool
	Path          string //文件根目录路径
	CodeReadOnly  string //只读权限code
	CodeReadWrite string //读写权限code
	LockUploadDir string //锁定上传目录
}

// APP:VAR
var (
	app    *engine.AppConfig
	appId  = "pass"
	config = new(AppConfig)

	uiPath string
	//go:embed ui/dist
	uiFS embed.FS
)

// APP:INIT
func init() {
	uiDist, _ := fs.Sub(uiFS, "ui/dist")
	app = &engine.AppConfig{
		Name:   appId,
		Type:   engine.APP_APP,
		Config: config,
		UIFS:   uiDist,
		Run:    run,
	}
	engine.AppInstall(app)
	uiPath = filepath.Join(app.Dir, "ui", "dist")
}

// APP:RUN
func run() {
	// 教师端默认密码 admin（可在 config.ini [pass] CodeReadWrite 修改）
	if strings.TrimSpace(config.CodeReadWrite) == "" {
		config.CodeReadWrite = "admin"
	}
	engine.Print(aurora.Green("App pass loaded"), aurora.BrightCyan(config))
	engine.Gin.Use(engine.CorsMiddleware())
	// 大文件 multipart：内存只留 32MB，其余落临时盘，避免整文件进内存
	engine.Gin.MaxMultipartMemory = 32 << 20
	initSessionStore()
	routeStatic(config.Live)
	routeApi()
	routeWs()
	putDll()
}
