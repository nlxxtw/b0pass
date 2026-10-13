package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
)

type Session struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Mode                 string `json:"mode"` // distribute | collect
	RequireIdentity      bool   `json:"requireIdentity"`
	CanSeeOthers         bool   `json:"canSeeOthers"`
	CanDownloadOthers    bool   `json:"canDownloadOthers"`
	CanDownloadMaterials bool   `json:"canDownloadMaterials"`
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
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	SizeText    string `json:"sizeText"`
	SubmittedAt string `json:"submittedAt,omitempty"`
	StudentID   string `json:"studentId,omitempty"`
	StudentName string `json:"studentName,omitempty"`
	Mine        bool   `json:"mine"`
	CanDownload bool   `json:"canDownload"`
	Kind        string `json:"kind"` // material | submission
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

func (s *Store) SessionDir(id string) string {
	return filepath.Join(s.root, "_np", id)
}

func (s *Store) MaterialsDir(id string) string {
	return filepath.Join(s.SessionDir(id), "materials")
}

func (s *Store) InboxDir(id string) string {
	return filepath.Join(s.SessionDir(id), "inbox")
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

func (s *Store) Create(name, mode string, requireIdentity, canSeeOthers, canDownloadOthers, canDownloadMaterials bool, deadline string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if name == "" {
		return nil, errors.New("名称不能为空")
	}
	if mode != ModeDistribute && mode != ModeCollect {
		return nil, errors.New("无效的活动类型")
	}
	if !canSeeOthers {
		canDownloadOthers = false
	}
	if mode == ModeDistribute {
		canDownloadMaterials = true
	}

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
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
	if err := os.MkdirAll(s.MaterialsDir(id), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.InboxDir(id), 0755); err != nil {
		return nil, err
	}
	meta, _ := json.MarshalIndent(sess, "", "  ")
	_ = os.WriteFile(filepath.Join(s.SessionDir(id), "meta.json"), meta, 0644)

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

func (s *Store) StudentFolder(sessionID, studentID, studentName, anonID string, requireIdentity bool) (string, string, error) {
	base := s.InboxDir(sessionID)
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
	dir := s.MaterialsDir(sess.ID)
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
		info, err := e.Info()
		if err != nil {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("_np", sess.ID, "materials", e.Name()))
		out = append(out, FileItem{
			Name:        e.Name(),
			Path:        rel,
			Size:        info.Size(),
			SizeText:    HumanSize(info.Size()),
			CanDownload: sess.CanDownloadMaterials,
			Kind:        "material",
			Mine:        false,
		})
	}
	return out, nil
}

func (s *Store) ListSubmissions(sess *Session, studentID, studentName, anonID string, forTeacher bool) ([]FileItem, error) {
	dir := s.InboxDir(sess.ID)
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
			rel := filepath.ToSlash(filepath.Join("_np", sess.ID, "inbox", folder, f.Name()))
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
	abs = filepath.Join(s.root, rel)
	// must stay under _np
	npRoot := filepath.Join(s.root, "_np")
	absClean := filepath.Clean(abs)
	if !strings.HasPrefix(absClean, filepath.Clean(npRoot)+string(os.PathSeparator)) && absClean != filepath.Clean(npRoot) {
		return "", errors.New("非法路径")
	}
	st, err := os.Stat(absClean)
	if err != nil || st.IsDir() {
		return "", errors.New("文件不存在")
	}
	return absClean, nil
}

func (s *Store) Stats(sess *Session) (map[string]interface{}, error) {
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
	return map[string]interface{}{
		"fileCount":      len(items),
		"studentCount":   len(people),
		"submissions":    items,
		"session":        sess,
	}, nil
}
