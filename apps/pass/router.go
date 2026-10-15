package app

import (
	"b0go/apps/pass/lib/chat"
	"b0go/core/engine"
	"io/fs"
	"net/http"
	"os"
	"path"

	"github.com/gin-gonic/gin"
)

func serveUIFile(live bool, name string, c *gin.Context) {
	if live {
		c.File(path.Join(uiPath, name))
		return
	}
	b, err := uiFS.ReadFile("ui/dist/" + name)
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", b)
}

func routeStatic(live bool) {
	engine.Gin.GET("/", func(c *gin.Context) {
		serveUIFile(live, "index.html", c)
	})
	engine.Gin.GET("/admin", func(c *gin.Context) {
		serveUIFile(live, "admin.html", c)
	})
	engine.Gin.GET("/admin/", func(c *gin.Context) {
		serveUIFile(live, "admin.html", c)
	})
	engine.Gin.GET("/login", func(c *gin.Context) {
		// 无口令模式：登录页直接等同管理端，避免与 /admin 互相跳转
		serveUIFile(live, "admin.html", c)
	})

	// /app/pass 由 gateway 统一挂载，这里只挂学生/老师页与静态资源，避免重复路由导致启动崩溃
	if live {
		engine.Gin.Static("/assets", path.Join(uiPath, "assets"))
	} else {
		assets, _ := fs.Sub(uiFS, "ui/dist/assets")
		engine.Gin.StaticFS("/assets", http.FS(assets))
	}

	if _, err := os.Stat(config.Path); err != nil {
		os.MkdirAll(config.Path, 0777)
	}
	filesGroup := engine.Gin.Group("/files")
	filesGroup.Use(JWTMiddleware("ro"))
	filesGroup.StaticFS("/", http.Dir(config.Path))
}

func routeWs() {
	hub := chat.NewHub()
	go hub.Run()
	engine.Gin.GET("/ws", func(c *gin.Context) {
		chat.ServeWs(hub, c)
	})
}

func routeApi() {
	GET("/login", "{code=权限code}", "login", Login)
	GET("/ping", "{}", "连通性测试", Ping)

	GET("/public/current", "{}", "当前发布活动", PublicCurrent)
	GET("/public/sessions", "{}", "学生活动列表", PublicSessions)
	GET("/public/files", "{id}", "学生文件列表", PublicFiles)
	GET("/public/download", "{id,f}", "学生下载", PublicDownload)
	GET("/public/version", "{}", "当前版本", PublicVersion)
	GET("/public/update-check", "{}", "检查更新", PublicUpdateCheck)
	engine.Gin.POST("/pass/public/upload", PublicUpload)
	engine.Gin.POST("/pass/public/delete-submission", PublicDeleteSubmission)
	// 学生端「打开」：在老师主机上打开作业所在文件夹
	engine.Gin.POST("/pass/public/reveal", PublicReveal)
	GET("/public/reveal", "{id,f}", "本机打开", PublicReveal)
	GET("/public/text/messages", "{id}", "发文本消息列表", PublicTextMessages)
	GET("/public/text/media", "{id,f}", "发文本图片", PublicTextMedia)
	engine.Gin.POST("/pass/public/text/upload", PublicTextUpload)
	engine.Gin.POST("/pass/public/text/send", PublicTextSend)
	engine.Gin.POST("/pass/public/text/delete", PublicTextDelete)

	GETX("/read-config", "{}", "读取配置(ro)", ReadConfig)
	GETX("/read-ip", "{}", "读取IP(ro)", ReadIP)
	GETX("/node-tree", "{}", "目录树结构(ro)", NodeTree)
	GETX("/file-count", "{f=相对路径}", "文件数量(ro)", FileCount)
	GETX("/file-list", "{f=相对路径,[t=需要的类型]}", "文件列表(ro)", FileList)
	GETX("/file-content", "{f=相对路径,文件名称}", "文件内容(ro)", FileContent)
	GETX("/file-download", "{f=相对路径}", "文件下载(ro)", FileDownload)

	GETX("/session/list", "{}", "活动列表(ro)", SessionList)
	GETX("/session/stats", "{id}", "活动统计(ro)", SessionStats)
	GETX("/session/folders", "{}", "可选目录(ro)", SessionFolders)
	POSTX("/session/create", "{}", "创建活动(rw)", SessionCreate)
	POSTX("/session/publish", "{id}", "发布活动(rw)", SessionPublish)
	POSTX("/session/delete", "{id}", "删除活动(rw)", SessionDelete)
	POSTX("/session/material-upload", "{id}", "上传资料(rw)", SessionMaterialUpload)
	POSTX("/session/pick-folder", "{}", "本机选目录(rw)", SessionPickFolder)
	POSTX("/session/capture-drop", "{}", "本机拖放识别路径(rw)", SessionCaptureDrop)
	POSTX("/session/resolve-path", "{path}", "校验本机路径(rw)", SessionResolvePath)
	POSTX("/session/set-folder", "{id,folder}", "修改活动目录(rw)", SessionSetFolder)
	POSTX("/session/reveal", "{id}", "本机打开文件夹(rw)", SessionReveal)
	POSTX("/session/delete-submission", "{id,f}", "删除学生作业(rw)", SessionDeleteSubmission)

	POSTX("/cmd-open", "{}", "命令行打开(rw)", CmdOpen)
	POSTX("/cmd-key", "{}", "主电脑键盘(rw)", CmdKey)
	POSTX("/node-add", "{f=相对路径}", "添加目录(rw)", NodeAdd)
	POSTX("/node-rename", "{f=原路径,n=新路径}", "重命名节点(rw)", NodeRename)
	POSTX("/node-delete", "{f=相对路径}", "删除节点(rw)", NodeRemove)
	POSTX("/file-upload", "{post file}", "大文件上传(rw)", FileUpload)
}

func GET(url, param, title string, handle ...gin.HandlerFunc) {
	engine.Router(appId, "GET", url, param, title, handle...)
}

func POST(url, param, title string, handle ...gin.HandlerFunc) {
	engine.Router(appId, "POST", url, param, title, handle...)
}

func GETX(url, param, title string, handle gin.HandlerFunc) {
	engine.Router(appId, "GET", url, param, "(Auth)"+title, JWTMiddleware("ro"), handle)
}

func POSTX(url, param, title string, handle gin.HandlerFunc) {
	engine.Router(appId, "POST", url, param, "(Auth)"+title, JWTMiddleware("rw"), handle)
}
