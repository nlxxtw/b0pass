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
		serveUIFile(live, "login.html", c)
	})

	if live {
		engine.Gin.Static("/assets", path.Join(uiPath, "assets"))
		engine.Gin.Static("/app/pass", uiPath)
	} else {
		uiDist, _ := fs.Sub(uiFS, "ui/dist")
		assets, _ := fs.Sub(uiFS, "ui/dist/assets")
		engine.Gin.StaticFS("/assets", http.FS(assets))
		engine.Gin.StaticFS("/app/pass", http.FS(uiDist))
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
	GET("/public/files", "{}", "学生文件列表", PublicFiles)
	GET("/public/download", "{f=相对路径}", "学生下载", PublicDownload)
	engine.Gin.POST("/pass/public/upload", PublicUpload)

	GETX("/read-config", "{}", "读取配置(ro)", ReadConfig)
	GETX("/read-ip", "{}", "读取IP(ro)", ReadIP)
	GETX("/node-tree", "{}", "目录树结构(ro)", NodeTree)
	GETX("/file-count", "{f=相对路径}", "文件数量(ro)", FileCount)
	GETX("/file-list", "{f=相对路径,[t=需要的类型]}", "文件列表(ro)", FileList)
	GETX("/file-content", "{f=相对路径,文件名称}", "文件内容(ro)", FileContent)
	GETX("/file-download", "{f=相对路径}", "文件下载(ro)", FileDownload)

	GETX("/session/list", "{}", "活动列表(ro)", SessionList)
	GETX("/session/stats", "{id}", "活动统计(ro)", SessionStats)
	POSTX("/session/create", "{}", "创建活动(rw)", SessionCreate)
	POSTX("/session/publish", "{id}", "发布活动(rw)", SessionPublish)
	POSTX("/session/material-upload", "{id}", "上传资料(rw)", SessionMaterialUpload)

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
