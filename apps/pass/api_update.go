package app

import (
	"b0go/core/engine"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ghRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
	HTMLURL string `json:"html_url"`
}

func mirrorURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, GithubMirror) {
		return raw
	}
	return GithubMirror + raw
}

func normalizeVer(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

// compareSemver 返回 1 if a>b, -1 if a<b, 0 if equal/unparsable-equal
func compareSemver(a, b string) int {
	a = normalizeVer(a)
	b = normalizeVer(b)
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(as) {
			ai, _ = strconv.Atoi(strings.TrimSpace(as[i]))
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(strings.TrimSpace(bs[i]))
		}
		if ai > bi {
			return 1
		}
		if ai < bi {
			return -1
		}
	}
	return 0
}

func assetNameForOS() string {
	switch runtime.GOOS {
	case "windows":
		return "nextpass-win.zip"
	case "darwin":
		return "nextpass-mac.zip"
	default:
		return "nextpass-linux.zip"
	}
}

func fetchLatestRelease() (*ghRelease, error) {
	api := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", GithubRepo)
	urls := []string{mirrorURL(api), api}
	var lastErr error
	client := &http.Client{Timeout: 12 * time.Second}
	for _, u := range urls {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "NextPass/"+AppVersion)
		res, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if res.StatusCode != 200 {
			lastErr = fmt.Errorf("github %d: %s", res.StatusCode, string(b))
			continue
		}
		var rel ghRelease
		if err := json.Unmarshal(b, &rel); err != nil {
			lastErr = err
			continue
		}
		if rel.TagName == "" {
			lastErr = fmt.Errorf("empty tag")
			continue
		}
		return &rel, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("无法获取版本信息")
	}
	return nil, lastErr
}

func pickDownloadURL(rel *ghRelease) string {
	want := assetNameForOS()
	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, want) {
			return mirrorURL(a.BrowserDownloadURL)
		}
	}
	// 兜底：镜像 releases 页
	if rel.HTMLURL != "" {
		return mirrorURL(rel.HTMLURL)
	}
	return mirrorURL(fmt.Sprintf("https://github.com/%s/releases/latest", GithubRepo))
}

// PublicUpdateCheck 检查 GitHub Releases 是否有新版本（强制更新用）
func PublicUpdateCheck(c *gin.Context) {
	cur := normalizeVer(AppVersion)
	out := gin.H{
		"current":     cur,
		"latest":      cur,
		"needUpdate":  false,
		"downloadUrl": "",
		"releaseUrl":  mirrorURL(fmt.Sprintf("https://github.com/%s/releases/latest", GithubRepo)),
		"notes":       "",
		"asset":       assetNameForOS(),
		"repo":        GithubRepo,
		"mirror":      GithubMirror,
	}
	rel, err := fetchLatestRelease()
	if err != nil {
		// 检查失败不强制拦死（避免无网完全不能用），前端可提示
		out["checkError"] = err.Error()
		engine.OK("OK", out, c)
		return
	}
	latest := normalizeVer(rel.TagName)
	out["latest"] = latest
	out["notes"] = rel.Body
	out["downloadUrl"] = pickDownloadURL(rel)
	if compareSemver(latest, cur) > 0 {
		out["needUpdate"] = true
	}
	engine.OK("OK", out, c)
}

// PublicVersion 当前版本
func PublicVersion(c *gin.Context) {
	engine.OK("OK", gin.H{
		"version": AppVersion,
		"repo":    GithubRepo,
		"mirror":  GithubMirror,
	}, c)
}
