package app

import (
	"b0go/apps/pass/lib/files"
	"b0go/apps/pass/session"
	"b0go/core/engine"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var sessStore *session.Store

func initSessionStore() {
	sessStore = session.NewStore(config.Path)
}

// Teacher: create session
func SessionCreate(c *gin.Context) {
	var body struct {
		Name                 string `json:"name"`
		Mode                 string `json:"mode"`
		RequireIdentity      bool   `json:"requireIdentity"`
		CanSeeOthers         bool   `json:"canSeeOthers"`
		CanDownloadOthers    bool   `json:"canDownloadOthers"`
		CanDownloadMaterials bool   `json:"canDownloadMaterials"`
		Deadline             string `json:"deadline"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		// also accept form query for simplicity
		body.Name = c.Query("name")
		body.Mode = c.DefaultQuery("mode", session.ModeCollect)
		body.RequireIdentity = c.Query("requireIdentity") == "1" || c.Query("requireIdentity") == "true"
		body.CanSeeOthers = c.Query("canSeeOthers") == "1" || c.Query("canSeeOthers") == "true"
		body.CanDownloadOthers = c.Query("canDownloadOthers") == "1" || c.Query("canDownloadOthers") == "true"
		body.CanDownloadMaterials = c.Query("canDownloadMaterials") != "0" && c.Query("canDownloadMaterials") != "false"
		body.Deadline = c.Query("deadline")
		if body.Name == "" {
			engine.ERR("参数错误", c)
			return
		}
	}
	if body.Mode == "" {
		body.Mode = session.ModeCollect
	}
	sess, err := sessStore.Create(body.Name, body.Mode, body.RequireIdentity, body.CanSeeOthers, body.CanDownloadOthers, body.CanDownloadMaterials, body.Deadline)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", sess, c)
}

func SessionList(c *gin.Context) {
	idx, err := sessStore.List()
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", idx, c)
}

func SessionPublish(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		id = c.Query("id")
	}
	if err := sessStore.Publish(id); err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", gin.H{"publishedId": id}, c)
}

func SessionStats(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		id = c.Query("id")
	}
	sess, err := sessStore.Get(id)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	stats, err := sessStore.Stats(sess)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", stats, c)
}

// Teacher upload material into current or specified session materials/
func SessionMaterialUpload(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		cur, err := sessStore.Current()
		if err != nil || cur == nil {
			engine.ERR("请先创建并发布活动", c)
			return
		}
		id = cur.ID
	}
	dir := sessStore.MaterialsDir(id)
	_ = os.MkdirAll(dir, 0755)
	lens := 0
	if v := c.Request.Header.Get("Content-Length"); v != "" {
		lens, _ = strconv.Atoi(v)
	}
	files.NodeAdd(dir)
	if lens > 4096 {
		FileUploadBig(dir+string(os.PathSeparator), c)
	} else {
		FileUploadTiny(dir+string(os.PathSeparator), c)
	}
}

// Public: current published session
func PublicCurrent(c *gin.Context) {
	sess, err := sessStore.Current()
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	if sess == nil {
		engine.OK("OK", gin.H{"session": nil, "closed": false}, c)
		return
	}
	engine.OK("OK", gin.H{
		"session":  sess,
		"expired":  sessStore.IsPastDeadline(sess),
		"closed":   false,
	}, c)
}

// Public: list files for student
func PublicFiles(c *gin.Context) {
	sess, err := sessStore.Current()
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	if sess == nil {
		engine.OK("OK", gin.H{"materials": []any{}, "submissions": []any{}}, c)
		return
	}
	sid := c.Query("studentId")
	sname := c.Query("studentName")
	anon := c.Query("anonId")

	materials := []session.FileItem{}
	if sess.CanDownloadMaterials || sess.Mode == session.ModeDistribute {
		materials, err = sessStore.ListMaterials(sess)
		if err != nil {
			engine.ERR(err.Error(), c)
			return
		}
	}
	submissions := []session.FileItem{}
	if sess.Mode == session.ModeCollect {
		submissions, err = sessStore.ListSubmissions(sess, sid, sname, anon, false)
		if err != nil {
			engine.ERR(err.Error(), c)
			return
		}
	}
	engine.OK("OK", gin.H{
		"materials":   materials,
		"submissions": submissions,
		"expired":     sessStore.IsPastDeadline(sess),
	}, c)
}

// Public download
func PublicDownload(c *gin.Context) {
	sess, err := sessStore.Current()
	if err != nil || sess == nil {
		engine.ERR("当前没有发布的活动", c)
		return
	}
	rel := c.Query("f")
	if rel == "" {
		engine.ERR("路径不能为空", c)
		return
	}
	relSlash := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	abs, err := sessStore.ResolveDownload(relSlash)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}

	sid := c.Query("studentId")
	sname := c.Query("studentName")
	anon := c.Query("anonId")

	allowed := false
	if strings.Contains(relSlash, "/materials/") {
		allowed = sess.CanDownloadMaterials || sess.Mode == session.ModeDistribute
	} else if strings.Contains(relSlash, "/inbox/") {
		items, _ := sessStore.ListSubmissions(sess, sid, sname, anon, false)
		for _, it := range items {
			if filepath.ToSlash(it.Path) == relSlash && it.CanDownload {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		engine.ERR("无权下载该文件", c)
		return
	}

	fileName := path.Base(abs)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Type", "application/octet-stream")
	c.File(abs)
}

// Public upload homework
func PublicUpload(c *gin.Context) {
	sess, err := sessStore.Current()
	if err != nil || sess == nil {
		engine.ERR("当前没有发布的活动", c)
		return
	}
	if sess.Mode != session.ModeCollect {
		engine.ERR("当前活动不接受上传", c)
		return
	}
	if sessStore.IsPastDeadline(sess) {
		engine.ERR("已过截止时间", c)
		return
	}

	sid := c.Query("studentId")
	sname := c.Query("studentName")
	anon := c.Query("anonId")
	if sid == "" {
		sid = c.PostForm("studentId")
	}
	if sname == "" {
		sname = c.PostForm("studentName")
	}
	if anon == "" {
		anon = c.PostForm("anonId")
	}

	folder, _, err := sessStore.StudentFolder(sess.ID, sid, sname, anon, sess.RequireIdentity)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}

	lens := 0
	if v := c.Request.Header.Get("Content-Length"); v != "" {
		lens, _ = strconv.Atoi(v)
	}
	dest := folder + string(os.PathSeparator)
	log.Println("PublicUpload:::", dest)
	if lens > 4096 {
		FileUploadBig(dest, c)
	} else {
		FileUploadTiny(dest, c)
	}

	// best-effort meta (filename may be unknown for multipart stream; update mtime folder)
	_ = sessStore.WriteSubmitMeta(folder, session.SubmitMeta{
		StudentID:   sid,
		StudentName: sname,
		SubmittedAt: time.Now().Format(time.RFC3339),
	})
}
