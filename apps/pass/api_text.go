package app

import (
	"b0go/apps/pass/session"
	"b0go/core/engine"
	"strings"

	"github.com/gin-gonic/gin"
)

func clientIPv4(c *gin.Context) string {
	ip := session.NormalizeIPv4(c.ClientIP())
	if ip != "" {
		return ip
	}
	// 回退：部分代理场景
	ip = session.NormalizeIPv4(c.GetHeader("X-Real-IP"))
	if ip != "" {
		return ip
	}
	xff := c.GetHeader("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		ip = session.NormalizeIPv4(strings.TrimSpace(parts[0]))
		if ip != "" {
			return ip
		}
	}
	return "未知"
}

func requireTextSession(id string) (*session.Session, error) {
	if id == "" {
		return nil, errText("缺少活动 id")
	}
	sess, err := sessStore.Get(id)
	if err != nil {
		return nil, err
	}
	if sess.Mode != session.ModeText {
		return nil, errText("不是发文本活动")
	}
	return sess, nil
}

type simpleErr string

func (e simpleErr) Error() string { return string(e) }

func errText(s string) error { return simpleErr(s) }

func isTeacherReq(c *gin.Context) bool {
	tok := c.GetHeader("token")
	if tok == "" {
		tok = c.Query("token")
	}
	if tok == "" {
		return false
	}
	// 必须是有效教师 JWT，不能只看 header 非空（学生页若残留 token 会误标教师）
	mc, err := engine.ParseToken(tok)
	if err != nil || mc == nil {
		return false
	}
	return true
}

// PublicTextMessages 学生/教师拉取消息
func PublicTextMessages(c *gin.Context) {
	id := c.Query("id")
	sess, err := requireTextSession(id)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	list, err := sessStore.ListTextMessages(sess.ID)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	view := publicSessionView(sess)
	if view != nil {
		view["teacherIp"] = strings.TrimSpace(sess.TeacherIP)
	}
	engine.OK("OK", gin.H{
		"session":   view,
		"messages":  list,
		"myNick":    clientIPv4(c),
		"teacherIp": strings.TrimSpace(sess.TeacherIP),
	}, c)
}

// PublicTextSend 发送文本（昵称=IPv4；教师标签绑定管理端首次发消息的 IP）
func PublicTextSend(c *gin.Context) {
	var body struct {
		ID   string `json:"id"`
		Body string `json:"body"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.ID == "" {
		body.ID = c.Query("id")
	}
	sess, err := requireTextSession(body.ID)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	ip := clientIPv4(c)
	role := "student"
	teacherIP := strings.TrimSpace(sess.TeacherIP)
	// 仅管理端（有效 JWT）可绑定教师 IP；之后「教师」标签只认该 IP
	if isTeacherReq(c) {
		bound, err2 := sessStore.BindTeacherIP(sess.ID, ip, true)
		if err2 == nil {
			teacherIP = bound
			sess.TeacherIP = bound
		}
	}
	if teacherIP != "" && ip == teacherIP {
		role = "teacher"
	}
	msg, err := sessStore.AddTextMessage(sess.ID, ip, body.Body, role)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("OK", msg, c)
}

// PublicTextDelete 教师（绑定 IP）可删任意消息；学生只能删自己的
func PublicTextDelete(c *gin.Context) {
	var body struct {
		ID    string `json:"id"`
		MsgID string `json:"msgId"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.ID == "" {
		body.ID = c.Query("id")
	}
	if body.MsgID == "" {
		body.MsgID = c.Query("msgId")
	}
	sess, err := requireTextSession(body.ID)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	msg, err := sessStore.GetTextMessage(sess.ID, body.MsgID)
	if err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	ip := clientIPv4(c)
	teacherIP := strings.TrimSpace(sess.TeacherIP)
	if isTeacherReq(c) {
		if bound, err2 := sessStore.BindTeacherIP(sess.ID, ip, true); err2 == nil {
			teacherIP = bound
		}
	}
	isTeacher := teacherIP != "" && ip == teacherIP
	if !isTeacher && msg.Nick != ip {
		engine.ERR("只能删除自己的消息", c)
		return
	}
	if err := sessStore.DeleteTextMessage(sess.ID, body.MsgID); err != nil {
		engine.ERR(err.Error(), c)
		return
	}
	engine.OK("已删除", gin.H{"id": body.MsgID}, c)
}
