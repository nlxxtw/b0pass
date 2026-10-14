package session

import (
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
	return html
}

func htmlLooksEmpty(html string) bool {
	plain := html
	plain = regexp.MustCompile(`(?is)<br\s*/?>`).ReplaceAllString(plain, "")
	plain = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(plain, "")
	plain = strings.ReplaceAll(plain, "\u00a0", " ")
	plain = strings.TrimSpace(plain)
	return plain == ""
}

func (s *Store) AddTextMessage(sessionID, nick, body, role string) (*TextMessage, error) {
	body = strings.TrimSpace(body)
	body = stripDangerousHTML(body)
	if body == "" || htmlLooksEmpty(body) {
		return nil, errors.New("内容不能为空")
	}
	// 富文本 HTML，限制长度
	runes := []rune(body)
	if len(runes) > 20000 {
		body = string(runes[:20000])
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
