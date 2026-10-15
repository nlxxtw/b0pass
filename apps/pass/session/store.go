package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	ModeDistribute = "distribute" // 发资料
	ModeCollect    = "collect"    // 收作业
	ModeText       = "text"       // 发文本
)

const (
	DirMaterials = "老师资料"
	DirInbox     = "学生提交"
)

type Session struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Folder               string `json:"folder"`              // 收作业：files 下相对名；若有 SharePath 则仅作显示名
	SharePath            string `json:"sharePath,omitempty"` // 发资料=资料夹；收作业=可选本机任意盘绝对路径
	Mode                 string `json:"mode"`                // distribute | collect | text
	RequireIdentity      bool   `json:"requireIdentity"`
	CanSeeOthers         bool   `json:"canSeeOthers"`
	CanDownloadOthers    bool   `json:"canDownloadOthers"`
	CanDownloadMaterials bool   `json:"canDownloadMaterials"`
	AccessPassword       string `json:"accessPassword,omitempty"` // 发资料可选访问密码；空=不设
	TeacherIP            string `json:"teacherIp,omitempty"`      // 发文本：教师机 IPv4，标签只绑此 IP
	Deadline             string `json:"deadline,omitempty"`
	CreatedAt            string `json:"createdAt"`
}

type Index struct {
	PublishedID string     `json:"publishedId"`
	Sessions    []*Session `json:"sessions"`
}

type SubmitMeta struct {
	StudentID   string `json:"studentId"`
	StudentName string `json:"studentName"`
	FileName    string `json:"fileName"`
	Size        int64  `json:"size"`
	SubmittedAt string `json:"submittedAt"`
}

type FileItem struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Size          int64  `json:"size"`
	SizeText      string `json:"sizeText"`
	SubmittedAt   string `json:"submittedAt,omitempty"`
	StudentID     string `json:"studentId,omitempty"`
	StudentName   string `json:"studentName,omitempty"`
	Mine          bool   `json:"mine"`
	CanDownload   bool   `json:"canDownload"`
	Kind          string `json:"kind"` // material | submission
	DownloadCount int    `json:"downloadCount,omitempty"`
}

// DownloadStats 按文件记录下载过的学生 IPv4；同一 IP 重复下载不计
type DownloadStats struct {
	IPs   map[string][]string `json:"ips"`             // fileName -> unique IPv4 list
	Total int                 `json:"total,omitempty"` // 兼容旧数据
	Files map[string]int      `json:"files,omitempty"` // 兼容旧数据（次数，无 IP）
}

type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

func newID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) npDir() string {
	return filepath.Join(s.root, "_nextpass")
}

func (s *Store) indexPath() string {
	return filepath.Join(s.npDir(), "sessions.json")
}

func (s *Store) SessionDir(sess *Session) string {
	if sess == nil {
		return s.absRoot()
	}
	// 收作业可指定任意盘绝对路径
	if sess.Mode == ModeCollect && strings.TrimSpace(sess.SharePath) != "" {
		return filepath.Clean(sess.SharePath)
	}
	if sess.Folder != "" {
		return filepath.Join(s.absRoot(), sess.Folder)
	}
	// 旧数据兼容
	return filepath.Join(s.absRoot(), "_np", sess.ID)
}

func (s *Store) MaterialsDir(sess *Session) string {
	// 发资料：SharePath 可为文件夹，或单个文件（此时返回其所在目录）
	if sess != nil && sess.Mode == ModeDistribute && strings.TrimSpace(sess.SharePath) != "" {
		p := filepath.Clean(sess.SharePath)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return filepath.Dir(p)
		}
		return p
	}
	return filepath.Join(s.SessionDir(sess), DirMaterials)
}

// ShareIsFile 发资料是否指向单个文件
func (s *Store) ShareIsFile(sess *Session) bool {
	if sess == nil || strings.TrimSpace(sess.SharePath) == "" {
		return false
	}
	st, err := os.Stat(filepath.Clean(sess.SharePath))
	return err == nil && !st.IsDir()
}

func (s *Store) InboxDir(sess *Session) string {
	return filepath.Join(s.SessionDir(sess), DirInbox)
}

func (s *Store) ensureDirs() error {
	return os.MkdirAll(s.npDir(), 0755)
}

func (s *Store) loadIndex() (*Index, error) {
	if err := s.ensureDirs(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Index{Sessions: []*Session{}}, nil
		}
		return nil, err
	}
	idx := &Index{}
	if err := json.Unmarshal(b, idx); err != nil {
		return nil, err
	}
	if idx.Sessions == nil {
		idx.Sessions = []*Session{}
	}
	return idx, nil
}

func (s *Store) saveIndex(idx *Index) error {
	if err := s.ensureDirs(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), b, 0644)
}

func (s *Store) Create(name, folder, sharePath, mode string, requireIdentity, canSeeOthers, canDownloadOthers, canDownloadMaterials bool, deadline, accessPassword string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if name == "" {
		return nil, errors.New("名称不能为空")
	}
	if mode != ModeDistribute && mode != ModeCollect && mode != ModeText {
		return nil, errors.New("无效的活动类型")
	}
	if !canSeeOthers {
		canDownloadOthers = false
	}

	id := newID(6)
	sess := &Session{
		ID:                   id,
		Name:                 name,
		Mode:                 mode,
		RequireIdentity:      requireIdentity,
		CanSeeOthers:         canSeeOthers,
		CanDownloadOthers:    canDownloadOthers,
		CanDownloadMaterials: canDownloadMaterials,
		Deadline:             deadline,
		CreatedAt:            time.Now().Format(time.RFC3339),
	}

	if mode == ModeDistribute {
		canDownloadMaterials = true
		sess.CanDownloadMaterials = true
		sharePath = filepath.Clean(strings.TrimSpace(sharePath))
		if sharePath == "" {
			return nil, errors.New("请选择要发给学生的文件或文件夹")
		}
		if _, err := os.Stat(sharePath); err != nil {
			return nil, errors.New("本地路径不存在")
		}
		// 允许目录或单个文件
		sess.SharePath = sharePath
		sess.Folder = ""
		sess.AccessPassword = strings.TrimSpace(accessPassword)
	} else if mode == ModeText {
		// 发文本：主题=名称，消息存 _nextpass/text/{id}.json
		sess.Folder = ""
		sess.SharePath = ""
		sess.RequireIdentity = false
		sess.CanSeeOthers = true
		sess.CanDownloadOthers = false
		sess.CanDownloadMaterials = false
		sess.AccessPassword = ""
		sess.Deadline = ""
		if err := os.MkdirAll(s.textDir(), 0755); err != nil {
			return nil, err
		}
		_ = s.saveTextMessages(id, []TextMessage{})
	} else {
		sharePath = filepath.Clean(strings.TrimSpace(sharePath))
		folderRaw := strings.TrimSpace(folder)
		// 输入框里直接贴绝对路径时，也按任意盘处理
		if sharePath == "" && filepath.IsAbs(folderRaw) {
			sharePath = filepath.Clean(folderRaw)
		}
		if sharePath != "" {
			if err := os.MkdirAll(sharePath, 0755); err != nil {
				return nil, fmt.Errorf("无法创建目录: %v", err)
			}
			st, err := os.Stat(sharePath)
			if err != nil || !st.IsDir() {
				return nil, errors.New("本地文件夹不存在或不是目录")
			}
			sess.SharePath = sharePath
			sess.Folder = filepath.Base(sharePath)
		} else {
			folder = SafeFolder(folderRaw)
			if folder == "" {
				folder = s.uniqueFolder(time.Now().Format("20060102"))
			} else if s.folderTaken(folder, "") {
				return nil, errors.New("目录已存在，请换一个或点「选择」")
			}
			sess.Folder = folder
			sess.SharePath = ""
		}
		if err := os.MkdirAll(s.MaterialsDir(sess), 0755); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(s.InboxDir(sess), 0755); err != nil {
			return nil, err
		}
		meta, _ := json.MarshalIndent(sess, "", "  ")
		_ = os.WriteFile(filepath.Join(s.SessionDir(sess), "meta.json"), meta, 0644)
	}

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	idx.Sessions = append([]*Session{sess}, idx.Sessions...)
	idx.PublishedID = id
	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) List() (*Index, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadIndex()
}

func (s *Store) Get(id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	for _, sess := range idx.Sessions {
		if sess.ID == id {
			return sess, nil
		}
	}
	return nil, errors.New("活动不存在")
}

func (s *Store) Publish(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	found := false
	for _, sess := range idx.Sessions {
		if sess.ID == id {
			found = true
			break
		}
	}
	if !found {
		return errors.New("活动不存在")
	}
	idx.PublishedID = id
	return s.saveIndex(idx)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	next := make([]*Session, 0, len(idx.Sessions))
	var victim *Session
	for _, sess := range idx.Sessions {
		if sess.ID == id {
			victim = sess
			continue
		}
		next = append(next, sess)
	}
	if victim == nil {
		return errors.New("活动不存在")
	}
	idx.Sessions = next
	if idx.PublishedID == id {
		idx.PublishedID = ""
		if len(next) > 0 {
			idx.PublishedID = next[0].ID
		}
	}
	// 发资料只删登记，不删老师本机原文件夹
	// 收作业若在任意盘：只清活动子目录，不删用户选中的父文件夹
	if victim.Mode == ModeText {
		s.removeTextMessages(victim.ID)
	} else if victim.Mode == ModeCollect && strings.TrimSpace(victim.SharePath) != "" {
		base := s.SessionDir(victim)
		_ = os.RemoveAll(filepath.Join(base, DirInbox))
		_ = os.RemoveAll(filepath.Join(base, DirMaterials))
		_ = os.Remove(filepath.Join(base, "meta.json"))
	} else if victim.Mode == ModeCollect || (victim.SharePath == "" && victim.Folder != "") {
		_ = os.RemoveAll(s.SessionDir(victim))
	} else if victim.Folder == "" && victim.SharePath == "" {
		_ = os.RemoveAll(s.SessionDir(victim))
	}
	_ = os.Remove(s.downloadsPath(victim))
	return s.saveIndex(idx)
}

func (s *Store) Current() (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	if idx.PublishedID == "" {
		return nil, nil
	}
	for _, sess := range idx.Sessions {
		if sess.ID == idx.PublishedID {
			return sess, nil
		}
	}
	return nil, nil
}

var safeRe = regexp.MustCompile(`[^a-zA-Z0-9\p{Han}_\-]+`)

func SafePart(v string) string {
	v = strings.TrimSpace(v)
	v = safeRe.ReplaceAllString(v, "_")
	if v == "" {
		v = "anon"
	}
	if len([]rune(v)) > 40 {
		r := []rune(v)
		v = string(r[:40])
	}
	return v
}

// SafeFolder 相对 Path 的目录，允许一层或多层（如 20261013）
func SafeFolder(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "\\", "/")
	parts := strings.Split(v, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." || p == ".." {
			continue
		}
		p = SafePart(p)
		if p == "" || p == "anon" {
			continue
		}
		out = append(out, p)
	}
	return filepath.Join(out...)
}

func (s *Store) folderTaken(folder string, exceptID string) bool {
	idx, err := s.loadIndex()
	if err == nil {
		for _, old := range idx.Sessions {
			if old.ID != exceptID && strings.EqualFold(old.Folder, folder) {
				return true
			}
		}
	}
	dir := filepath.Join(s.root, folder)
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return false
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		return true // 非空目录视为占用
	}
	return false
}

func (s *Store) uniqueFolder(base string) string {
	base = SafeFolder(base)
	if base == "" {
		base = time.Now().Format("20060102")
	}
	if !s.folderTaken(base, "") {
		return base
	}
	// 同日冲突：追加时分
	cand := base + "_" + time.Now().Format("1504")
	if !s.folderTaken(cand, "") {
		return cand
	}
	return base + "_" + newID(3)
}

func (s *Store) absRoot() string {
	root, err := filepath.Abs(filepath.Clean(s.root))
	if err != nil {
		return filepath.Clean(s.root)
	}
	return filepath.Clean(root)
}

// RelFromAbs 把本机绝对路径转为 Path 下相对目录。
// 注意 config.Path 常为相对路径 "files"，必须先 Abs 再比较，否则选对也会报错。
func (s *Store) RelFromAbs(abs string) (string, error) {
	abs = filepath.Clean(abs)
	if a, err := filepath.Abs(abs); err == nil {
		abs = a
	}
	root := s.absRoot()
	sep := string(os.PathSeparator)
	if strings.EqualFold(abs, root) {
		// 对话框停在 files 根目录点确定：用当天目录名
		return time.Now().Format("20060102"), nil
	}
	prefix := strings.ToLower(root + sep)
	if !strings.HasPrefix(strings.ToLower(abs), prefix) {
		return "", errors.New("请选择 files 目录下的文件夹（当前根目录：" + root + "）")
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	rel = SafeFolder(rel)
	if rel == "" {
		return "", errors.New("无效目录")
	}
	return rel, nil
}

func (s *Store) ListFolders() ([]string, error) {
	ents, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	out := []string{}
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}

func (s *Store) SetFolder(id, folder string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	folder = SafeFolder(folder)
	if folder == "" {
		return nil, errors.New("目录不能为空")
	}
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	var sess *Session
	for _, it := range idx.Sessions {
		if it.ID == id {
			sess = it
			break
		}
	}
	if sess == nil {
		return nil, errors.New("活动不存在")
	}
	if strings.EqualFold(sess.Folder, folder) {
		return sess, nil
	}
	for _, it := range idx.Sessions {
		if it.ID != id && strings.EqualFold(it.Folder, folder) {
			return nil, errors.New("目录已被其他活动使用")
		}
	}

	oldDir := s.SessionDir(sess)
	newDir := filepath.Join(s.root, folder)
	if st, err := os.Stat(newDir); err == nil && st.IsDir() {
		ents, _ := os.ReadDir(newDir)
		if len(ents) > 0 {
			return nil, errors.New("目标目录已有内容，请换一个")
		}
		_ = os.Remove(newDir)
	}
	if err := os.MkdirAll(filepath.Dir(newDir), 0755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(oldDir); err == nil {
		if err := os.Rename(oldDir, newDir); err != nil {
			return nil, fmt.Errorf("移动目录失败: %v", err)
		}
	} else {
		if err := os.MkdirAll(filepath.Join(newDir, DirMaterials), 0755); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Join(newDir, DirInbox), 0755); err != nil {
			return nil, err
		}
	}
	sess.Folder = folder
	meta, _ := json.MarshalIndent(sess, "", "  ")
	_ = os.WriteFile(filepath.Join(newDir, "meta.json"), meta, 0644)
	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) StudentFolder(sessionID, studentID, studentName, anonID string, requireIdentity bool) (string, string, error) {
	sess, err := s.Get(sessionID)
	if err != nil {
		return "", "", err
	}
	base := s.InboxDir(sess)
	var folderName string
	if requireIdentity {
		if studentID == "" || studentName == "" {
			return "", "", errors.New("请填写学号和姓名")
		}
		folderName = SafePart(studentID) + "_" + SafePart(studentName)
	} else {
		if anonID == "" {
			anonID = newID(8)
		}
		folderName = "anon_" + SafePart(anonID)
	}
	folder := filepath.Join(base, folderName)
	if err := os.MkdirAll(folder, 0755); err != nil {
		return "", "", err
	}
	return folder, folderName, nil
}

func (s *Store) relUnder(sess *Session, parts ...string) string {
	base := sess.Folder
	if base == "" {
		base = filepath.ToSlash(filepath.Join("_np", sess.ID))
	}
	all := append([]string{base}, parts...)
	return filepath.ToSlash(filepath.Join(all...))
}

// NeedPassword 发资料是否设置了访问密码
func (s *Store) NeedPassword(sess *Session) bool {
	return sess != nil && sess.Mode == ModeDistribute && strings.TrimSpace(sess.AccessPassword) != ""
}

// CheckAccessPassword 校验发资料访问密码；未设密码直接通过
func (s *Store) CheckAccessPassword(sess *Session, pwd string) error {
	if !s.NeedPassword(sess) {
		return nil
	}
	if strings.TrimSpace(pwd) == "" {
		return errors.New("请输入访问密码")
	}
	if strings.TrimSpace(pwd) != strings.TrimSpace(sess.AccessPassword) {
		return errors.New("访问密码错误")
	}
	return nil
}

func (s *Store) IsPastDeadline(sess *Session) bool {
	if sess == nil || sess.Deadline == "" {
		return false
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, sess.Deadline, time.Local); err == nil {
			return time.Now().After(t)
		}
	}
	return false
}

func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}

func (s *Store) ListMaterials(sess *Session) ([]FileItem, error) {
	// 单个文件：只列出这一份
	if s.ShareIsFile(sess) {
		p := filepath.Clean(sess.SharePath)
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(p)
		rel := filepath.ToSlash(filepath.Join("share", sess.ID, name))
		return []FileItem{{
			Name:        name,
			Path:        rel,
			Size:        info.Size(),
			SizeText:    HumanSize(info.Size()),
			CanDownload: sess.CanDownloadMaterials || sess.Mode == ModeDistribute,
			Kind:        "material",
			Mine:        false,
		}}, nil
	}

	dir := s.MaterialsDir(sess)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []FileItem{}, nil
		}
		return nil, err
	}
	out := []FileItem{}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		if strings.HasSuffix(e.Name(), ".npuploading") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		var rel string
		if sess.SharePath != "" {
			rel = filepath.ToSlash(filepath.Join("share", sess.ID, e.Name()))
		} else {
			rel = s.relUnder(sess, DirMaterials, e.Name())
		}
		out = append(out, FileItem{
			Name:        e.Name(),
			Path:        rel,
			Size:        info.Size(),
			SizeText:    HumanSize(info.Size()),
			CanDownload: sess.CanDownloadMaterials || sess.Mode == ModeDistribute,
			Kind:        "material",
			Mine:        false,
		})
	}
	return out, nil
}

// PathAllowed 校验目标是否属于该活动可打开的本机目录
func (s *Store) PathAllowed(sess *Session, target string) bool {
	if sess == nil {
		return false
	}
	target = filepath.Clean(target)
	if a, err := filepath.Abs(target); err == nil {
		target = a
	}
	sep := string(os.PathSeparator)
	under := func(root string) bool {
		root = filepath.Clean(root)
		if root == "" {
			return false
		}
		if a, err := filepath.Abs(root); err == nil {
			root = a
		}
		if strings.EqualFold(target, root) {
			return true
		}
		return strings.HasPrefix(strings.ToLower(target), strings.ToLower(root+sep))
	}
	if sess.SharePath != "" && under(sess.SharePath) {
		return true
	}
	if under(s.SessionDir(sess)) {
		return true
	}
	if under(s.MaterialsDir(sess)) || under(s.InboxDir(sess)) {
		return true
	}
	if under(s.root) {
		return true
	}
	return false
}

func (s *Store) ResolveUnder(root, name string) (string, error) {
	root = filepath.Clean(root)
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "", errors.New("文件名无效")
	}
	abs := filepath.Clean(filepath.Join(root, name))
	sep := string(os.PathSeparator)
	if abs != root && !strings.HasPrefix(abs, root+sep) {
		return "", errors.New("非法路径")
	}
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		return "", errors.New("文件不存在")
	}
	return abs, nil
}

func (s *Store) ResolveSessionFile(sess *Session, rel string) (string, error) {
	relSlash := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if sess != nil && sess.Mode == ModeDistribute && sess.SharePath != "" {
		sp := filepath.Clean(sess.SharePath)
		if st, err := os.Stat(sp); err == nil && !st.IsDir() {
			if filepath.Base(relSlash) == filepath.Base(sp) {
				return sp, nil
			}
			return "", errors.New("文件不存在")
		}
		return s.ResolveUnder(sp, filepath.Base(relSlash))
	}
	if sess != nil && sess.Mode == ModeCollect {
		if strings.Contains(relSlash, "/"+DirInbox+"/") || strings.HasPrefix(relSlash, DirInbox+"/") {
			return s.ResolveSubmissionFile(sess, relSlash)
		}
	}
	return s.ResolveDownload(relSlash)
}

func (s *Store) studentDirName(sess *Session, studentID, studentName, anonID string) string {
	if sess == nil {
		return ""
	}
	if sess.RequireIdentity {
		if studentID == "" || studentName == "" {
			return ""
		}
		return SafePart(studentID) + "_" + SafePart(studentName)
	}
	if anonID == "" {
		return ""
	}
	return "anon_" + SafePart(anonID)
}

// MySubmitInfo 当前学生是否已交及最近提交时间（不创建目录）
func (s *Store) MySubmitInfo(sess *Session, studentID, studentName, anonID string) (bool, string) {
	name := s.studentDirName(sess, studentID, studentName, anonID)
	if name == "" {
		return false, ""
	}
	dir := filepath.Join(s.InboxDir(sess), name)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false, ""
	}
	hasFile := false
	var newest time.Time
	for _, e := range ents {
		if e.IsDir() || e.Name() == "_submit.json" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		hasFile = true
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if !hasFile {
		return false, ""
	}
	at := ""
	if b, err := os.ReadFile(filepath.Join(dir, "_submit.json")); err == nil {
		var meta SubmitMeta
		if json.Unmarshal(b, &meta) == nil && meta.SubmittedAt != "" {
			if t, err := time.Parse(time.RFC3339, meta.SubmittedAt); err == nil {
				at = t.Format("01-02 15:04")
				if t.After(newest) {
					newest = t
				}
			} else {
				at = meta.SubmittedAt
			}
		}
	}
	if at == "" && !newest.IsZero() {
		at = newest.Format("01-02 15:04")
	}
	return true, at
}

// ResolveSubmissionFile 解析收作业文件（兼容 files 相对路径与任意盘 SharePath）
func (s *Store) ResolveSubmissionFile(sess *Session, relSlash string) (string, error) {
	relSlash = filepath.ToSlash(filepath.Clean(filepath.FromSlash(relSlash)))
	if relSlash == "" || strings.Contains(relSlash, "..") {
		return "", errors.New("非法路径")
	}
	parts := strings.Split(relSlash, "/")
	idx := -1
	for i, p := range parts {
		if p == DirInbox {
			idx = i
			break
		}
	}
	var abs string
	if idx >= 0 {
		rest := parts[idx:]
		abs = filepath.Clean(filepath.Join(append([]string{s.SessionDir(sess)}, rest...)...))
	} else if sess != nil && strings.TrimSpace(sess.SharePath) != "" {
		abs = filepath.Clean(filepath.Join(sess.SharePath, filepath.FromSlash(relSlash)))
	} else {
		return s.ResolveDownload(relSlash)
	}
	if sess != nil && !s.PathAllowed(sess, abs) {
		return "", errors.New("非法路径")
	}
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		return "", errors.New("文件不存在")
	}
	return abs, nil
}

// DeleteSubmissionFile 删除一份提交；学生仅可删自己的。文件夹空则清理。
func (s *Store) DeleteSubmissionFile(sess *Session, rel, studentID, studentName, anonID string, asTeacher bool) error {
	relSlash := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	abs, err := s.ResolveSubmissionFile(sess, relSlash)
	if err != nil {
		return err
	}
	if !asTeacher {
		items, err2 := s.ListSubmissions(sess, studentID, studentName, anonID, false)
		if err2 != nil {
			return err2
		}
		ok := false
		for _, it := range items {
			if filepath.ToSlash(it.Path) == relSlash && it.Mine {
				ok = true
				break
			}
		}
		if !ok {
			return errors.New("只能删除自己的作业")
		}
	}
	if err := os.Remove(abs); err != nil {
		return err
	}
	dir := filepath.Dir(abs)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	empty := true
	for _, e := range ents {
		if e.Name() == "_submit.json" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		empty = false
		break
	}
	if empty {
		_ = os.RemoveAll(dir)
	}
	return nil
}

func (s *Store) ListSubmissions(sess *Session, studentID, studentName, anonID string, forTeacher bool) ([]FileItem, error) {
	dir := s.InboxDir(sess)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []FileItem{}, nil
		}
		return nil, err
	}

	myFolder := ""
	if sess.RequireIdentity && studentID != "" {
		myFolder = SafePart(studentID) + "_" + SafePart(studentName)
	} else if anonID != "" {
		myFolder = "anon_" + SafePart(anonID)
	}

	out := []FileItem{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		folder := e.Name()
		mine := myFolder != "" && folder == myFolder
		if !forTeacher {
			if !sess.CanSeeOthers && !mine {
				continue
			}
		}
		sid, sname := parseFolderIdentity(folder)
		files, err := os.ReadDir(filepath.Join(dir, folder))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || f.Name() == "_submit.json" || strings.HasPrefix(f.Name(), ".") {
				continue
			}
			// 上传未完成的临时文件不进列表（避免半截/0B 被当成已提交）
			if strings.HasSuffix(f.Name(), ".npuploading") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			submittedAt := info.ModTime().Format("01-02 15:04")
			metaPath := filepath.Join(dir, folder, "_submit.json")
			if b, err := os.ReadFile(metaPath); err == nil {
				var meta SubmitMeta
				if json.Unmarshal(b, &meta) == nil && meta.SubmittedAt != "" {
					if t, err := time.Parse(time.RFC3339, meta.SubmittedAt); err == nil {
						submittedAt = t.Format("01-02 15:04")
					}
					if meta.StudentID != "" {
						sid = meta.StudentID
					}
					if meta.StudentName != "" {
						sname = meta.StudentName
					}
				}
			}
			canDl := forTeacher || mine || sess.CanDownloadOthers
			rel := s.relUnder(sess, DirInbox, folder, f.Name())
			out = append(out, FileItem{
				Name:        f.Name(),
				Path:        rel,
				Size:        info.Size(),
				SizeText:    HumanSize(info.Size()),
				SubmittedAt: submittedAt,
				StudentID:   sid,
				StudentName: sname,
				Mine:        mine,
				CanDownload: canDl,
				Kind:        "submission",
			})
		}
	}
	return out, nil
}

func parseFolderIdentity(folder string) (id, name string) {
	if strings.HasPrefix(folder, "anon_") {
		return "", "匿名"
	}
	parts := strings.SplitN(folder, "_", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return folder, ""
}

func (s *Store) WriteSubmitMeta(folder string, meta SubmitMeta) error {
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(folder, "_submit.json"), b, 0644)
}

func (s *Store) ResolveDownload(rel string) (abs string, err error) {
	rel = filepath.Clean(filepath.FromSlash(rel))
	if strings.Contains(rel, "..") {
		return "", errors.New("非法路径")
	}
	root := s.absRoot()
	absClean := filepath.Clean(filepath.Join(root, rel))
	sep := string(os.PathSeparator)
	if absClean != root && !strings.HasPrefix(strings.ToLower(absClean), strings.ToLower(root+sep)) {
		return "", errors.New("非法路径")
	}
	st, err := os.Stat(absClean)
	if err != nil || st.IsDir() {
		return "", errors.New("文件不存在")
	}
	return absClean, nil
}

func (s *Store) downloadsPath(sess *Session) string {
	return filepath.Join(s.npDir(), "dl", sess.ID+".json")
}

func (s *Store) ReadDownloads(sess *Session) *DownloadStats {
	out := &DownloadStats{IPs: map[string][]string{}, Files: map[string]int{}}
	if sess == nil {
		return out
	}
	b, err := os.ReadFile(s.downloadsPath(sess))
	if err != nil {
		return out
	}
	_ = json.Unmarshal(b, out)
	if out.IPs == nil {
		out.IPs = map[string][]string{}
	}
	if out.Files == nil {
		out.Files = map[string]int{}
	}
	return out
}

func (st *DownloadStats) FileUniqueCount(fileName string) int {
	if st == nil {
		return 0
	}
	if ips, ok := st.IPs[fileName]; ok && len(ips) > 0 {
		return len(ips)
	}
	// 旧数据：只有次数没有 IP
	return st.Files[fileName]
}

func (st *DownloadStats) UniqueIPTotal() int {
	if st == nil {
		return 0
	}
	set := map[string]struct{}{}
	for _, ips := range st.IPs {
		for _, ip := range ips {
			if ip != "" {
				set[ip] = struct{}{}
			}
		}
	}
	if len(set) > 0 {
		return len(set)
	}
	// 旧数据兜底
	n := 0
	for _, c := range st.Files {
		n += c
	}
	if n == 0 {
		n = st.Total
	}
	return n
}

// NormalizeIPv4 提取客户端 IPv4；非 IPv4 返回空
func NormalizeIPv4(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// 可能带端口
	if host, _, err := splitHostPortSafe(raw); err == nil {
		raw = host
	}
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	raw = strings.TrimPrefix(raw, "::ffff:")
	ip := net.ParseIP(raw)
	if ip == nil {
		return ""
	}
	v4 := ip.To4()
	if v4 == nil {
		return ""
	}
	return v4.String()
}

func splitHostPortSafe(hostport string) (string, string, error) {
	if strings.Count(hostport, ":") == 1 || strings.HasPrefix(hostport, "[") {
		return net.SplitHostPort(hostport)
	}
	return hostport, "", fmt.Errorf("no port")
}

// RecordDownloadIP 按 IPv4 去重记录；同一文件同一 IP 只计 1 次。返回是否为新记录。
// 写盘放在锁外：杀软扫描/磁盘卡顿时不能拖住全局 mu，否则全班进不了学生端。
func (s *Store) RecordDownloadIP(sess *Session, fileName, ip string) bool {
	ip = NormalizeIPv4(ip)
	if sess == nil || fileName == "" || ip == "" {
		return false
	}
	path := s.downloadsPath(sess)
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	s.mu.Lock()
	st := s.ReadDownloads(sess)
	if st.IPs == nil {
		st.IPs = map[string][]string{}
	}
	if st.Files == nil {
		st.Files = map[string]int{}
	}
	for _, exist := range st.IPs[fileName] {
		if exist == ip {
			s.mu.Unlock()
			return false
		}
	}
	st.IPs[fileName] = append(st.IPs[fileName], ip)
	st.Files[fileName] = len(st.IPs[fileName])
	st.Total = st.UniqueIPTotal()
	b, err := json.MarshalIndent(st, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return false
	}
	_ = os.WriteFile(path, b, 0644)
	return true
}

func (s *Store) SetSharePath(id, sharePath string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sharePath = filepath.Clean(strings.TrimSpace(sharePath))
	if sharePath == "" {
		return nil, errors.New("请选择本地文件或文件夹")
	}
	st, err := os.Stat(sharePath)
	if err != nil {
		return nil, errors.New("本地路径不存在")
	}
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	var sess *Session
	for _, it := range idx.Sessions {
		if it.ID == id {
			sess = it
			break
		}
	}
	if sess == nil {
		return nil, errors.New("活动不存在")
	}
	if sess.Mode != ModeDistribute && sess.Mode != ModeCollect {
		return nil, errors.New("无效活动")
	}
	if sess.Mode == ModeCollect {
		if !st.IsDir() {
			return nil, errors.New("收作业请选择文件夹")
		}
		if err := os.MkdirAll(sharePath, 0755); err != nil {
			return nil, fmt.Errorf("无法创建目录: %v", err)
		}
		sess.SharePath = sharePath
		sess.Folder = filepath.Base(sharePath)
		_ = os.MkdirAll(filepath.Join(sharePath, DirMaterials), 0755)
		_ = os.MkdirAll(filepath.Join(sharePath, DirInbox), 0755)
		meta, _ := json.MarshalIndent(sess, "", "  ")
		_ = os.WriteFile(filepath.Join(sharePath, "meta.json"), meta, 0644)
	} else {
		// 发资料：文件或文件夹均可
		sess.SharePath = sharePath
	}
	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) Stats(sess *Session) (map[string]interface{}, error) {
	out := map[string]interface{}{
		"session": sess,
	}
	if sess.Mode == ModeDistribute {
		materials, err := s.ListMaterials(sess)
		if err != nil {
			return nil, err
		}
		dl := s.ReadDownloads(sess)
		for i := range materials {
			materials[i].DownloadCount = dl.FileUniqueCount(materials[i].Name)
		}
		out["materials"] = materials
		out["materialCount"] = len(materials)
		out["downloadCount"] = dl.UniqueIPTotal()
		out["fileCount"] = len(materials)
		out["studentCount"] = dl.UniqueIPTotal()
		out["submissions"] = []FileItem{}
		return out, nil
	}
	if sess.Mode == ModeText {
		msgs, err := s.ListTextMessages(sess.ID)
		if err != nil {
			return nil, err
		}
		out["messages"] = msgs
		out["messageCount"] = len(msgs)
		out["fileCount"] = len(msgs)
		out["studentCount"] = 0
		out["submissions"] = []FileItem{}
		out["materials"] = []FileItem{}
		return out, nil
	}

	items, err := s.ListSubmissions(sess, "", "", "", true)
	if err != nil {
		return nil, err
	}
	people := map[string]bool{}
	for _, it := range items {
		key := it.StudentID + "|" + it.StudentName
		if key == "|" {
			key = it.Path
		}
		people[key] = true
	}
	out["fileCount"] = len(items)
	out["studentCount"] = len(people)
	out["submissions"] = items
	out["materials"] = []FileItem{}
	out["materialCount"] = 0
	out["downloadCount"] = 0
	return out, nil
}
