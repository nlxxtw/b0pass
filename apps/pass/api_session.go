package app

import (
	"b0go/apps/pass/lib/folderdialog"
	"b0go/apps/pass/session"
	"b0go/core/engine"
	"b0go/core/tools/cmd"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
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
		Folder               string `json:"folder"`
		SharePath            string `json:"sharePath"`
		Mode                 string `json:"mode"`
		RequireIdentity      bool   `json:"requireIdentity"`
		CanSeeOthers         bool   `json:"canSeeOthers"`
		CanDownloadOthers    bool   `json:"canDownloadOthers"`
		CanDownloadMaterials bool   `json:"canDownloadMaterials"`
		Deadline             string `json:"deadline"`
		AccessPassword       string `json:"accessPassword"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.Name = c.Query("name")
		body.Folder = c.Query("folder")
		body.SharePath = c.Query("sharePath")
		body.Mode = c.DefaultQuery("mode", session.ModeCollect)
		body.RequireIdentity = c.Query("requireIdentity") == "1" || c.Query("requireIdentity") == "true"
		body.CanSeeOthers = c.Query("canSeeOthers") == "1" || c.Query("canSeeOthers") == "true"
		body.CanDownloadOthers = c.Query("canDownloadOthers") == "1" || c.Query("canDownloadOthers") == "true"
		body.CanDownloadMaterials = c.Query("canDownloadMaterials") != "0" && c.Query("canDownloadMaterials") != "false"
		body.Deadline = c.Query("deadline")
		body.AccessPassword = c.Query("accessPassword")
		if body.Name == "" {
			engine.ERR("参数错误", c)
			return
		}
	}
	if body.Mode == "" {
		body.Mode = session.ModeCollect
	}
	sess, err := sessStore.Create(body.Name, body.Folder, body.SharePath, body.Mode, body.RequireIdentity, body.CanSeeOthers, body.CanDownloadOthers, body.CanDownloadMaterials, body.Deadline, body.AccessPassword)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	// 发文本：创建时就把教师标签绑到管理端当前 IPv4
	if sess != nil && sess.Mode == session.ModeText {
		if bound, err2 := sessStore.BindTeacherIP(sess.ID, clientIPv4(c), true); err2 == nil {
			sess.TeacherIP = bound
		}
	}
	engine.OK("OK", sess, c)
}

func accessPwdFrom(c *gin.Context) string {
	pwd := c.Query("pwd")
	if pwd == "" {
		pwd = c.GetHeader("X-Access-Password")
	}
	return pwd
}

func publicSessionView(s *session.Session) gin.H {
	if s == nil {
		return nil
	}
	return gin.H{
		"id":                   s.ID,
		"name":                 s.Name,
		"folder":               s.Folder,
		"mode":                 s.Mode,
		"requireIdentity":      s.RequireIdentity,
		"canSeeOthers":         s.CanSeeOthers,
		"canDownloadOthers":    s.CanDownloadOthers,
		"canDownloadMaterials": s.CanDownloadMaterials,
		"deadline":             s.Deadline,
		"createdAt":            s.CreatedAt,
		"teacherIp":            strings.TrimSpace(s.TeacherIP),
		"needPassword":         sessStore.NeedPassword(s),
		"expired":              sessStore.IsPastDeadline(s),
	}
}

func SessionList(c *gin.Context) {
	idx, err := sessStore.List()
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	// 发资料附带唯一 IP 下载人数，方便列表直接显示
	list := make([]gin.H, 0, len(idx.Sessions))
	for _, s := range idx.Sessions {
		item := gin.H{
			"id":                   s.ID,
			"name":                 s.Name,
			"folder":               s.Folder,
			"sharePath":            s.SharePath,
			"mode":                 s.Mode,
			"requireIdentity":      s.RequireIdentity,
			"canSeeOthers":         s.CanSeeOthers,
			"canDownloadOthers":    s.CanDownloadOthers,
			"canDownloadMaterials": s.CanDownloadMaterials,
			"deadline":             s.Deadline,
			"createdAt":            s.CreatedAt,
			"downloadCount":        0,
		}
		if s.Mode == session.ModeDistribute {
			item["downloadCount"] = sessStore.ReadDownloads(s).UniqueIPTotal()
			item["needPassword"] = sessStore.NeedPassword(s)
		}
		if s.Mode == session.ModeText {
			item["messageCount"] = sessStore.TextMessageCount(s.ID)
		}
		list = append(list, item)
	}
	engine.OK("OK", gin.H{"sessions": list, "publishedId": idx.PublishedID}, c)
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

func SessionDelete(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		engine.ERR("缺少活动 id", c)
		return
	}
	if err := sessStore.Delete(id); err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", gin.H{"deletedId": id}, c)
}

// SessionDeleteSubmission 教师删除某份学生作业（便于学生重交）
func SessionDeleteSubmission(c *gin.Context) {
	id := c.Query("id")
	var body struct {
		ID string `json:"id"`
		F  string `json:"f"`
	}
	_ = c.ShouldBindJSON(&body)
	if id == "" {
		id = body.ID
	}
	rel := body.F
	if rel == "" {
		rel = c.Query("f")
	}
	if id == "" || rel == "" {
		engine.ERR("缺少参数", c)
		return
	}
	sess, err := sessStore.Get(id)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	if err := sessStore.DeleteSubmissionFile(sess, rel, "", "", "", true); err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("已删除", gin.H{"deleted": rel}, c)
}

// PublicDeleteSubmission 学生删除自己的已交作业
func PublicDeleteSubmission(c *gin.Context) {
	sess, err := resolvePublicSession(c)
	if err != nil || sess == nil {
		engine.ERR("活动不存在", c)
		return
	}
	if sess.Mode != session.ModeCollect {
		engine.ERR("当前活动无作业可删", c)
		return
	}
	var body struct {
		F string `json:"f"`
	}
	_ = c.ShouldBindJSON(&body)
	rel := body.F
	if rel == "" {
		rel = c.Query("f")
	}
	if rel == "" {
		engine.ERR("缺少文件", c)
		return
	}
	sid := c.Query("studentId")
	sname := c.Query("studentName")
	anon := c.Query("anonId")
	if err := sessStore.DeleteSubmissionFile(sess, rel, sid, sname, anon, false); err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("已删除，可重新提交", gin.H{"deleted": rel}, c)
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
	stats["inboxPath"] = sessStore.InboxDir(sess)
	stats["materialsPath"] = sessStore.MaterialsDir(sess)
	stats["sharePath"] = sess.SharePath
	stats["folder"] = sess.Folder
	stats["rootPath"] = config.Path
	engine.OK("OK", stats, c)
}

func SessionFolders(c *gin.Context) {
	list, err := sessStore.ListFolders()
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", gin.H{
		"root":    config.Path,
		"folders": list,
		"today":   time.Now().Format("20060102"),
	}, c)
}

// SessionCaptureDrop 本机拖放识别路径（不上传、不拷贝，只填绝对路径）。
// 文件填文件路径，文件夹填文件夹路径。
func SessionCaptureDrop(c *gin.Context) {
	var body struct {
		X int `json:"x"`
		Y int `json:"y"`
		W int `json:"w"`
		H int `json:"h"`
	}
	_ = c.ShouldBindJSON(&body)
	folderdialog.ClearCaptureDropFlags()
	picked, err := folderdialog.CaptureDrop(20*time.Second, folderdialog.DropRect{
		X: body.X, Y: body.Y, W: body.W, H: body.H,
	})
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	if picked == "" {
		engine.OK("OK", gin.H{"abs": "", "sharePath": "", "cancelled": true}, c)
		return
	}
	share, wasFile, err := normalizeLocalSharePath(picked)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", gin.H{
		"abs":       share,
		"sharePath": share,
		"folder":    filepath.Base(share),
		"wasFile":   wasFile,
		"source":    picked,
	}, c)
}

// SessionResolvePath 校验/规范化本机路径（粘贴或浏览器能读到的路径）。
func SessionResolvePath(c *gin.Context) {
	var body struct {
		Path string `json:"path"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Path == "" {
		body.Path = c.Query("path")
	}
	share, wasFile, err := normalizeLocalSharePath(body.Path)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", gin.H{
		"abs":       share,
		"sharePath": share,
		"folder":    filepath.Base(share),
		"wasFile":   wasFile,
		"source":    strings.TrimSpace(body.Path),
	}, c)
}

func normalizeLocalSharePath(raw string) (share string, wasFile bool, err error) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	raw = strings.TrimPrefix(raw, "\ufeff")
	if raw == "" {
		return "", false, errors.New("路径为空")
	}
	// file:///C:/... → C:\...
	if strings.HasPrefix(strings.ToLower(raw), "file:") {
		u := raw
		u = strings.TrimPrefix(u, "file://")
		u = strings.TrimPrefix(u, "file:")
		u = strings.TrimPrefix(u, "//")
		if idx := strings.Index(u, "?"); idx >= 0 {
			u = u[:idx]
		}
		if unesc, e := url.QueryUnescape(u); e == nil {
			u = unesc
		}
		u = strings.ReplaceAll(u, "/", `\`)
		if strings.HasPrefix(u, `\`) && len(u) > 2 && u[2] == ':' {
			u = u[1:]
		}
		if strings.HasPrefix(strings.ToLower(u), `localhost\`) {
			u = u[len(`localhost\`):]
		}
		raw = u
	}
	abs, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", false, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", false, errors.New("路径不存在：" + abs)
	}
	// 文件就返回文件路径，文件夹返回文件夹路径（不再偷偷改成父目录）
	if st.IsDir() {
		return abs, false, nil
	}
	return abs, true, nil
}

// SessionPickFolder 本机弹窗选择。
// any=1：任意盘；any=0：仅 files 下。
// kind=folder|file|any：只选文件夹 / 只选文件 / 询问文件或文件夹（发资料用 any）。
func SessionPickFolder(c *gin.Context) {
	q := c.Query("any")
	anyLocal := q != "0" && q != "false"
	kind := strings.ToLower(strings.TrimSpace(c.Query("kind")))
	if kind == "" {
		kind = "folder"
	}
	start := ""
	title := "选择本机文件夹（可任选磁盘）"
	if anyLocal {
		start, _ = os.UserHomeDir()
	} else {
		title = "选择 files 下的目录"
		absRoot, err := filepath.Abs(config.Path)
		if err != nil {
			absRoot = config.Path
		}
		_ = os.MkdirAll(absRoot, 0755)
		start = absRoot
	}

	var picked string
	var err error
	switch kind {
	case "file":
		picked, err = folderdialog.PickFile(start, "选择本机文件")
	case "any", "both":
		picked, err = folderdialog.PickFileOrFolder(start)
	default:
		picked, err = folderdialog.Pick(start, title)
	}
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	if picked == "" {
		engine.ERR("未选择", c)
		return
	}

	share, wasFile, err := normalizeLocalSharePath(picked)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	if !anyLocal {
		rel, err := sessStore.RelFromAbs(share)
		if err != nil {
			engine.ERR(err.Error(), c)
			return
		}
		engine.OK("OK", gin.H{"folder": rel, "abs": share, "wasFile": wasFile, "source": picked}, c)
		return
	}
	engine.OK("OK", gin.H{
		"abs":       share,
		"sharePath": share,
		"folder":    filepath.Base(share),
		"wasFile":   wasFile,
		"source":    picked,
	}, c)
}

// PublicReveal 学生端查看：打开老师主机上的本地路径
func PublicReveal(c *gin.Context) {
	SessionReveal(c)
}

// SessionReveal 在本机打开文件夹 / 选中文件（不是浏览器下载）
func SessionReveal(c *gin.Context) {
	id := c.Query("id")
	var body struct {
		ID   string `json:"id"`
		F    string `json:"f"`
		Dir  bool   `json:"dir"`
		Path string `json:"path"` // 直接传绝对路径时仅允许活动目录内
	}
	_ = c.ShouldBindJSON(&body)
	if id == "" {
		id = body.ID
	}
	if id == "" {
		engine.ERR("缺少活动 id", c)
		return
	}
	sess, err := sessStore.Get(id)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}

	var target string
	if body.F != "" || c.Query("f") != "" {
		rel := body.F
		if rel == "" {
			rel = c.Query("f")
		}
		abs, err2 := sessStore.ResolveSessionFile(sess, rel)
		if err2 != nil {
			abs, err2 = sessStore.ResolveSubmissionFile(sess, filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))))
		}
		if err2 != nil {
			engine.ERR(err2.Error(), c)
			return
		}
		target = abs
		if body.Dir || c.Query("dir") == "1" || c.Query("dir") == "true" {
			target = filepath.Dir(abs)
		}
	} else if body.Path != "" {
		target = filepath.Clean(body.Path)
	} else if sess.Mode == session.ModeDistribute && sess.SharePath != "" {
		target = sess.SharePath
	} else {
		target = sessStore.InboxDir(sess)
	}

	absTarget, _ := filepath.Abs(target)
	if absTarget != "" {
		target = absTarget
	}
	if !sessStore.PathAllowed(sess, target) {
		engine.ERR("路径不在本活动目录内: "+target, c)
		return
	}
	if _, err := os.Stat(target); err != nil {
		engine.ERR("本地路径不存在: "+target, c)
		return
	}
	log.Println("SessionReveal:", target)
	// 本地目录/文件用 explorer，避免 cmd start 对中文路径偶发失败
	if err := cmd.Reveal(target); err != nil {
		engine.ERR("打开失败: "+err.Error(), c)
		return
	}
	engine.OK("已在主电脑打开", gin.H{"opened": target}, c)
}

func SessionSetFolder(c *gin.Context) {
	id := c.Query("id")
	var body struct {
		Folder    string `json:"folder"`
		SharePath string `json:"sharePath"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Folder == "" {
		body.Folder = c.Query("folder")
	}
	if body.SharePath == "" {
		body.SharePath = c.Query("sharePath")
	}
	if id == "" {
		engine.ERR("缺少 id", c)
		return
	}
	if body.SharePath != "" {
		sess, err := sessStore.SetSharePath(id, body.SharePath)
		if err != nil {
			engine.ERR(err.Error(), c)
			return
		}
		engine.OK("OK", gin.H{
			"session":       sess,
			"sharePath":     sess.SharePath,
			"materialsPath": sessStore.MaterialsDir(sess),
		}, c)
		return
	}
	if body.Folder == "" {
		engine.ERR("缺少目录", c)
		return
	}
	sess, err := sessStore.SetFolder(id, body.Folder)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", gin.H{
		"session":       sess,
		"inboxPath":     sessStore.InboxDir(sess),
		"materialsPath": sessStore.MaterialsDir(sess),
		"folder":        sess.Folder,
	}, c)
}

// Teacher upload material into current or specified session materials/
func SessionMaterialUpload(c *gin.Context) {
	id := c.Query("id")
	var sess *session.Session
	var err error
	if id == "" {
		sess, err = sessStore.Current()
		if err != nil || sess == nil {
			engine.ERR("请先创建活动", c)
			return
		}
	} else {
		sess, err = sessStore.Get(id)
		if err != nil {
			engine.ERR(err.Error(), c)
			return
		}
	}
	dir := sessStore.MaterialsDir(sess)
	// 与原版 FileUpload 相同：大文件走 FileUploadBig 流式
	passUploadToDir(dir, c)
}

func resolvePublicSession(c *gin.Context) (*session.Session, error) {
	id := c.Query("id")
	if id != "" {
		return sessStore.Get(id)
	}
	return sessStore.Current()
}

// Public: current published session (兼容旧前端)
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
		"session": sess,
		"expired": sessStore.IsPastDeadline(sess),
		"closed":  false,
	}, c)
}

// PublicSessions 学生端活动列表（全部可见，可点选展开）
func PublicSessions(c *gin.Context) {
	idx, err := sessStore.List()
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	sid := c.Query("studentId")
	sname := c.Query("studentName")
	anon := c.Query("anonId")
	list := make([]gin.H, 0, len(idx.Sessions))
	for _, s := range idx.Sessions {
		item := gin.H{
			"id":                   s.ID,
			"name":                 s.Name,
			"folder":               s.Folder,
			"mode":                 s.Mode,
			"requireIdentity":      s.RequireIdentity,
			"canSeeOthers":         s.CanSeeOthers,
			"canDownloadOthers":    s.CanDownloadOthers,
			"canDownloadMaterials": s.CanDownloadMaterials,
			"deadline":             s.Deadline,
			"createdAt":            s.CreatedAt,
			"expired":              sessStore.IsPastDeadline(s),
			"needPassword":         sessStore.NeedPassword(s),
			"submitted":            false,
			"submittedAt":          "",
		}
		if s.Mode == session.ModeCollect {
			ok, at := sessStore.MySubmitInfo(s, sid, sname, anon)
			item["submitted"] = ok
			item["submittedAt"] = at
		}
		if s.Mode == session.ModeText {
			item["messageCount"] = sessStore.TextMessageCount(s.ID)
		}
		list = append(list, item)
	}
	engine.OK("OK", gin.H{
		"sessions":    list,
		"publishedId": idx.PublishedID,
	}, c)
}

// Public: list files for student
func PublicFiles(c *gin.Context) {
	sess, err := resolvePublicSession(c)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	if sess == nil {
		engine.OK("OK", gin.H{"materials": []any{}, "submissions": []any{}}, c)
		return
	}
	if err := sessStore.CheckAccessPassword(sess, accessPwdFrom(c)); err != nil {
		engine.ERR(err.Error(), c)
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
		if sess.Mode == session.ModeDistribute {
			dl := sessStore.ReadDownloads(sess)
			for i := range materials {
				materials[i].DownloadCount = dl.FileUniqueCount(materials[i].Name)
			}
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
		"session":     publicSessionView(sess),
		"materials":   materials,
		"submissions": submissions,
		"expired":     sessStore.IsPastDeadline(sess),
	}, c)
}

// Public download
func PublicDownload(c *gin.Context) {
	sess, err := resolvePublicSession(c)
	if err != nil || sess == nil {
		engine.ERR("活动不存在", c)
		return
	}
	rel := c.Query("f")
	if rel == "" {
		engine.ERR("路径不能为空", c)
		return
	}
	if err := sessStore.CheckAccessPassword(sess, accessPwdFrom(c)); err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	relSlash := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	sid := c.Query("studentId")
	sname := c.Query("studentName")
	anon := c.Query("anonId")

	var abs string
	var isMaterial bool
	if sess.Mode == session.ModeDistribute {
		var err2 error
		abs, err2 = sessStore.ResolveSessionFile(sess, relSlash)
		if err2 != nil {
			engine.ERR(err2.Error(), c)
			return
		}
		isMaterial = true
	} else if strings.Contains(relSlash, "/"+session.DirMaterials+"/") || strings.Contains(relSlash, "/materials/") || strings.HasPrefix(relSlash, "share/") {
		var err2 error
		abs, err2 = sessStore.ResolveSessionFile(sess, relSlash)
		if err2 != nil {
			engine.ERR(err2.Error(), c)
			return
		}
		if !(sess.CanDownloadMaterials || sess.Mode == session.ModeDistribute) {
			engine.ERR("无权下载该文件", c)
			return
		}
		isMaterial = true
	} else if strings.Contains(relSlash, "/"+session.DirInbox+"/") || strings.Contains(relSlash, "/inbox/") {
		items, _ := sessStore.ListSubmissions(sess, sid, sname, anon, false)
		ok := false
		for _, it := range items {
			if filepath.ToSlash(it.Path) == relSlash && it.CanDownload {
				ok = true
				break
			}
		}
		if !ok {
			engine.ERR("无权下载该文件", c)
			return
		}
		var err2 error
		abs, err2 = sessStore.ResolveSubmissionFile(sess, relSlash)
		if err2 != nil {
			engine.ERR(err2.Error(), c)
			return
		}
	} else {
		engine.ERR("无权下载该文件", c)
		return
	}

	// Windows 路径必须用 filepath.Base；path.Base 会把整段盘符路径当文件名
	fileName := filepath.Base(abs)
	if isMaterial {
		// Safari 等会 Range 分段请求：只在首次/从 0 开始时记一次；同 IPv4 去重
		rng := c.GetHeader("Range")
		if rng == "" || strings.HasPrefix(rng, "bytes=0-") {
			ip := c.ClientIP()
			if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
				ip = strings.TrimSpace(strings.Split(xff, ",")[0])
			}
			// 异步记次数，避免写盘/杀软拖住下载与学生端页面
			go sessStore.RecordDownloadIP(sess, fileName, ip)
		}
	}
	serveAttachment(c, abs, fileName)
}

// rfc5987Encode 供 Safari 使用的 filename* 编码（按字节百分号编码）
func rfc5987Encode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func asciiFallbackName(fileName string) string {
	var ascii []rune
	for _, r := range fileName {
		if r < 128 && r > 31 && r != '"' && r != '\\' && r != '/' {
			ascii = append(ascii, r)
		}
	}
	fallback := string(ascii)
	if strings.Trim(fallback, "._- ") == "" {
		ext := filepath.Ext(fileName)
		if ext == "" {
			ext = ".bin"
		}
		return "file" + ext
	}
	return fallback
}

// serveAttachment 适配 Safari：强制附件下载 + RFC5987 中文名 + Range
func serveAttachment(c *gin.Context, abs, fileName string) {
	fileName = path.Base(strings.ReplaceAll(fileName, "\\", "/"))
	if fileName == "" || fileName == "." || fileName == ".." {
		fileName = "file.bin"
	}
	fallback := asciiFallbackName(fileName)
	disp := fmt.Sprintf(
		`attachment; filename="%s"; filename*=UTF-8''%s`,
		fallback,
		rfc5987Encode(fileName),
	)
	// 一律 octet-stream，避免 Safari 按 MIME 预览而不落盘
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", disp)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Accept-Ranges", "bytes")

	f, err := os.Open(abs)
	if err != nil {
		engine.ERR("文件不存在", c)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		engine.ERR("文件不存在", c)
		return
	}

	cw := &countingWriter{ResponseWriter: c.Writer}
	c.Writer = cw
	t0 := time.Now()
	http.ServeContent(c.Writer, c.Request, fileName, st.ModTime(), f)
	extra := fmt.Sprintf("fileSize=%s status=%d", humanBytes(st.Size()), c.Writer.Status())
	if rng := c.GetHeader("Range"); rng != "" {
		extra += " range=" + rng
	}
	logTransfer("DOWNLOAD", fileName, clientIPv4(c), cw.n, time.Since(t0), extra)
}

// passUploadToDir 学生/资料上传。
// 注意：不能对 Windows 绝对路径走 cleanPath/path.Join（会截断盘符路径），
// 也不能在未校验可靠时用旧 stream 解析——半截文件会被删掉表现为「闪一下没了」。
// 这里用 Gin FormFile + 临时文件改名，大文件由 multipart 落盘，与原版「能传完」一致。
func passUploadToDir(dir string, c *gin.Context) {
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Println("passUploadToDir mkdir fail:", dir, err)
		engine.ERR("无法创建上传目录", c)
		return
	}

	t0 := time.Now()
	// FormFile 会等到 multipart 收完（含网络传输时间）
	file, err := c.FormFile("file")
	recvCost := time.Since(t0)
	if err != nil {
		log.Println("passUploadToDir FormFile:", err)
		engine.ERR("请先选择文件", c)
		return
	}
	name := filepath.Base(file.Filename)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	if name == "" || name == "." || name == ".." {
		engine.ERR("文件名无效", c)
		return
	}

	final := filepath.Join(dir, name)
	tmp := final + uploadTmpSuffix
	_ = os.Remove(tmp)

	log.Printf("passUploadToDir:::: size=%d name=%s dir=%s\n", file.Size, name, dir)
	tSave := time.Now()
	if err := c.SaveUploadedFile(file, tmp); err != nil {
		_ = os.Remove(tmp)
		log.Println("passUploadToDir SaveUploadedFile:", err)
		engine.ERR("上传失败: "+err.Error(), c)
		return
	}
	saveCost := time.Since(tSave)

	st, err := os.Stat(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		engine.ERR("上传失败: 无法读取临时文件", c)
		return
	}
	if file.Size > 0 && st.Size() != file.Size {
		log.Printf("passUploadToDir size mismatch: got=%d want=%d\n", st.Size(), file.Size)
		_ = os.Remove(tmp)
		engine.ERR("上传不完整，请重新上传", c)
		return
	}
	if st.Size() == 0 && file.Size > 0 {
		_ = os.Remove(tmp)
		engine.ERR("上传失败: 文件为空", c)
		return
	}

	_ = os.Remove(final)
	if err := os.Rename(tmp, final); err != nil {
		// Windows 上偶发占用：退化成拷贝
		in, errOpen := os.Open(tmp)
		if errOpen != nil {
			_ = os.Remove(tmp)
			engine.ERR("保存文件失败: "+err.Error(), c)
			return
		}
		out, errCreate := os.Create(final)
		if errCreate != nil {
			in.Close()
			_ = os.Remove(tmp)
			engine.ERR("保存文件失败: "+errCreate.Error(), c)
			return
		}
		_, copyErr := io.Copy(out, in)
		in.Close()
		out.Close()
		_ = os.Remove(tmp)
		if copyErr != nil {
			_ = os.Remove(final)
			engine.ERR("保存文件失败: "+copyErr.Error(), c)
			return
		}
	}

	total := time.Since(t0)
	extra := fmt.Sprintf(
		"recv=%s save=%s dir=%s",
		recvCost.Round(time.Millisecond),
		saveCost.Round(time.Millisecond),
		dir,
	)
	// 吞吐按「接收完成」时间算（含网络）；save 多为本机拷贝
	logTransfer("UPLOAD", name, clientIPv4(c), st.Size(), recvCost, extra+" total="+total.Round(time.Millisecond).String())
	log.Printf("passUploadToDir OK:::: %s (%d bytes)\n", final, st.Size())
	engine.OK("上传成功", gin.H{"name": name, "size": st.Size()}, c)
}

// Public upload homework
func PublicUpload(c *gin.Context) {
	sess, err := resolvePublicSession(c)
	if err != nil || sess == nil {
		engine.ERR("请选择要提交的作业", c)
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

	// 直接落盘，不写 _submit.json（学号姓名已在文件夹名里，时间用文件修改时间）
	passUploadToDir(folder, c)
}
