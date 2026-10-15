package session

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// TextMessage 发文本活动中的一条消息
type TextMessage struct {
	ID        string `json:"id"`
	Nick      string `json:"nick"` // IPv4
	Body      string `json:"body"`
	Role      string `json:"role"` // teacher | student
	CreatedAt string `json:"createdAt"`
}

func (s *Store) textDir() string {
	return filepath.Join(s.npDir(), "text")
}

func (s *Store) textPath(sessionID string) string {
	return filepath.Join(s.textDir(), sessionID+".json")
}

func (s *Store) loadTextMessages(sessionID string) ([]TextMessage, error) {
	b, err := os.ReadFile(s.textPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return []TextMessage{}, nil
		}
		return nil, err
	}
	var list []TextMessage
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []TextMessage{}
	}
	return list, nil
}

func (s *Store) saveTextMessages(sessionID string, list []TextMessage) error {
	if err := os.MkdirAll(s.textDir(), 0755); err != nil {
		return err
	}
	if list == nil {
		list = []TextMessage{}
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.textPath(sessionID), b, 0644)
}

func (s *Store) ListTextMessages(sessionID string) ([]TextMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadTextMessages(sessionID)
}

func stripDangerousHTML(html string) string {
	// 局域网场景允许富文本；去掉明显危险标签（Go RE2 不支持反向引用）
	tags := []string{"script", "iframe", "object", "embed", "link", "meta", "form"}
	for _, t := range tags {
		re := regexp.MustCompile(`(?is)<` + t + `\b[^>]*>.*?</` + t + `\s*>`)
		html = re.ReplaceAllString(html, "")
		re2 := regexp.MustCompile(`(?is)<` + t + `\b[^>]*/?>`)
		html = re2.ReplaceAllString(html, "")
	}
	re3 := regexp.MustCompile(`(?i)\son[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	html = re3.ReplaceAllString(html, "")
	// 禁止 javascript: / vbscript: 图片地址
	re4 := regexp.MustCompile(`(?i)\ssrc\s*=\s*([\"']?)\s*(javascript|vbscript|data\s*:\s*text)\s*:`)
	html = re4.ReplaceAllString(html, ` src=$1#blocked:`)
	return html
}

func htmlLooksEmpty(html string) bool {
	low := strings.ToLower(html)
	// 纯截图/图片也算有内容
	if strings.Contains(low, "<img") || strings.Contains(low, "<video") {
		return false
	}
	plain := html
	plain = regexp.MustCompile(`(?is)<br\s*/?>`).ReplaceAllString(plain, "")
	plain = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(plain, "")
	plain = strings.ReplaceAll(plain, "\u00a0", " ")
	plain = strings.TrimSpace(plain)
	return plain == ""
}

func (s *Store) textMediaDir(sessionID string) string {
	return filepath.Join(s.textDir(), "media", sessionID)
}

// SaveTextMedia 保存聊天图片，返回可嵌入 HTML 的相对 URL
func (s *Store) SaveTextMedia(sessionID, ext string, data []byte) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("缺少活动 id")
	}
	if len(data) == 0 {
		return "", errors.New("空文件")
	}
	if len(data) > 12<<20 {
		return "", errors.New("图片不能超过 12MB")
	}
	ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	switch ext {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp":
	default:
		ext = "png"
	}
	dir := s.textMediaDir(sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name := newID(10) + "." + ext
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return "/pass/public/text/media?id=" + sessionID + "&f=" + name, nil
}

// ResolveTextMedia 校验并返回媒体绝对路径
func (s *Store) ResolveTextMedia(sessionID, file string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	file = filepath.Base(strings.TrimSpace(file))
	if sessionID == "" || file == "" || file == "." || file == ".." {
		return "", errors.New("无效文件")
	}
	if strings.ContainsAny(file, `/\`) {
		return "", errors.New("无效文件名")
	}
	abs := filepath.Join(s.textMediaDir(sessionID), file)
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		return "", errors.New("文件不存在")
	}
	return abs, nil
}

// Go RE2 不支持反向引用，双引号/单引号分开匹配
var (
	reDataImgDQ = regexp.MustCompile(`(?is)(<img\b[^>]*?\bsrc\s*=\s*)"(data:image/([a-z0-9.+-]+);base64,([A-Za-z0-9+/=\s]+))"`)
	reDataImgSQ = regexp.MustCompile(`(?is)(<img\b[^>]*?\bsrc\s*=\s*)'(data:image/([a-z0-9.+-]+);base64,([A-Za-z0-9+/=\s]+))'`)
)

func (s *Store) replaceDataImg(sessionID, html string, re *regexp.Regexp, quote string) string {
	return re.ReplaceAllStringFunc(html, func(m string) string {
		sub := re.FindStringSubmatch(m)
		// 1=prefix 2=dataURL 3=mime 4=b64
		if len(sub) < 5 {
			return m
		}
		prefix, mimeExt, b64 := sub[1], sub[3], sub[4]
		raw, err := base64.StdEncoding.DecodeString(strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, b64))
		if err != nil || len(raw) == 0 {
			return m
		}
		ext := strings.ToLower(mimeExt)
		if i := strings.IndexByte(ext, '+'); i >= 0 {
			ext = ext[:i]
		}
		if ext == "jpeg" {
			ext = "jpg"
		}
		url, err := s.SaveTextMedia(sessionID, ext, raw)
		if err != nil {
			return m
		}
		return prefix + quote + url + quote
	})
}

// materializeDataImages 把粘贴的 base64 图落到磁盘，避免 JSON 被截断成空气泡
func (s *Store) materializeDataImages(sessionID, html string) string {
	html = s.replaceDataImg(sessionID, html, reDataImgDQ, `"`)
	html = s.replaceDataImg(sessionID, html, reDataImgSQ, `'`)
	return html
}

func (s *Store) AddTextMessage(sessionID, nick, body, role string) (*TextMessage, error) {
	body = strings.TrimSpace(body)
	body = stripDangerousHTML(body)
	// 先落盘图片再判空/限长（纯截图可发送）
	body = s.materializeDataImages(sessionID, body)
	if body == "" || htmlLooksEmpty(body) {
		return nil, errors.New("内容不能为空")
	}
	// 富文本 HTML（图片已是短 URL）；纯文字/格式消息限制
	runes := []rune(body)
	if len(runes) > 100000 {
		return nil, errors.New("内容过长，请删减后再发")
	}
	nick = strings.TrimSpace(nick)
	if nick == "" {
		nick = "未知"
	}
	if role != "teacher" {
		role = "student"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadTextMessages(sessionID)
	if err != nil {
		return nil, err
	}
	msg := TextMessage{
		ID:        newID(8),
		Nick:      nick,
		Body:      body,
		Role:      role,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	list = append(list, msg)
	if err := s.saveTextMessages(sessionID, list); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (s *Store) GetTextMessage(sessionID, msgID string) (*TextMessage, error) {
	msgID = strings.TrimSpace(msgID)
	if msgID == "" {
		return nil, errors.New("缺少消息 id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadTextMessages(sessionID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == msgID {
			m := list[i]
			return &m, nil
		}
	}
	return nil, errors.New("消息不存在")
}

func (s *Store) DeleteTextMessage(sessionID, msgID string) error {
	msgID = strings.TrimSpace(msgID)
	if msgID == "" {
		return errors.New("缺少消息 id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadTextMessages(sessionID)
	if err != nil {
		return err
	}
	next := make([]TextMessage, 0, len(list))
	found := false
	for _, m := range list {
		if m.ID == msgID {
			found = true
			continue
		}
		next = append(next, m)
	}
	if !found {
		return errors.New("消息不存在")
	}
	return s.saveTextMessages(sessionID, next)
}

func (s *Store) removeTextMessages(sessionID string) {
	_ = os.Remove(s.textPath(sessionID))
}

// BindTeacherIP 发文本活动绑定教师机 IP。
// force=true：管理端发消息时更新为当前教师机 IP（标签只认此 IP）。
func (s *Store) BindTeacherIP(sessionID, ip string, force bool) (string, error) {
	ip = NormalizeIPv4(ip)
	if ip == "" {
		return "", errors.New("无效的教师 IP")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return "", err
	}
	var sess *Session
	for _, it := range idx.Sessions {
		if it.ID == sessionID {
			sess = it
			break
		}
	}
	if sess == nil {
		return "", errors.New("活动不存在")
	}
	if sess.Mode != ModeText {
		return "", errors.New("不是发文本活动")
	}
	cur := strings.TrimSpace(sess.TeacherIP)
	if force || cur == "" {
		if cur != ip {
			sess.TeacherIP = ip
			if err := s.saveIndex(idx); err != nil {
				return "", err
			}
		}
	}
	return sess.TeacherIP, nil
}

func (s *Store) TextMessageCount(sessionID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadTextMessages(sessionID)
	if err != nil {
		return 0
	}
	return len(list)
}
