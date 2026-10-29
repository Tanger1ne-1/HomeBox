package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"nas/internal/config"
	"nas/internal/files"
	"nas/internal/thumbs"
	"nas/internal/web"
)

// New 组装手机端 HTTP 处理器（路由 + 外网开关 + 鉴权 + 日志）。
func New(app *App) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/version", handleVersion)
	mux.HandleFunc("POST /api/login", app.handleLogin)
	mux.HandleFunc("POST /api/logout", app.handleLogout)
	mux.HandleFunc("GET /api/ping", app.handlePing)
	mux.HandleFunc("GET /api/list", app.handleList)
	mux.HandleFunc("GET /api/file", app.handleFile)
	mux.HandleFunc("GET /api/thumb", app.handleThumb)
	mux.HandleFunc("POST /api/upload", app.handleUpload)
	mux.HandleFunc("POST /api/delete", app.handleDelete)
	mux.HandleFunc("POST /api/move", app.handleMove)
	mux.HandleFunc("POST /api/create", app.handleCreate)
	mux.HandleFunc("POST /api/rename", app.handleRename)
	mux.HandleFunc("POST /api/copy", app.handleCopy)
	mux.HandleFunc("POST /api/hidden", app.handleHidden)
	mux.HandleFunc("GET /api/search", app.handleSearch)
	mux.HandleFunc("POST /api/save", app.handleSave)
	mux.HandleFunc("POST /api/sleep", app.handleSleep)
	mux.HandleFunc("GET /api/chat/state", app.handleChatState)
	mux.HandleFunc("GET /api/chat/messages", app.handleChatMessages)
	mux.HandleFunc("POST /api/chat/text", app.handleChatText)
	mux.HandleFunc("POST /api/chat/upload", app.handleChatUpload)
	mux.Handle("/", web.Handler())

	return app.authMiddleware(app.externalGate(withLogging(mux)))
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{
		"name":       "nasd",
		"version":    "0.2.0",
		"apiVersion": "1",
	})
}

// handleLogin 校验账号密码，成功后下发会话 Cookie。
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	token, err := a.Login(req.Username, req.Password)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.Logout(tokenFromRequest(r))
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, map[string]any{"ok": true})
}

// handlePing 是手机端心跳接口：页面保持打开时周期调用，刷新会话在线状态。
func (a *App) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
}

// authMiddleware 在已设置账号密码时，对所有 /api/*（除 login/version）要求登录。
func (a *App) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !a.RequiresAuth() || p == "/api/login" || p == "/api/version" || !strings.HasPrefix(p, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if !a.validMobileSession(tokenFromRequest(r)) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// externalGate 在外网开关关闭时，拒绝公网来源的请求（局域网仍可访问）。
func (a *App) externalGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.ExternalEnabled() && isGlobal(remoteIP(r)) {
			http.Error(w, "external access disabled", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) handleList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	// 隐藏文件夹：管理员账号可见（带闭眼标记），访客不可见。
	showHidden := a.isAdminAccount(r)
	if a.IsHiddenPath(path) && !showHidden {
		http.NotFound(w, r)
		return
	}
	canon, entries, err := files.List(a.RootDir(), path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !showHidden {
		entries = filterHidden(a, canon, entries)
	}
	resp := map[string]any{
		"path":    canon,
		"entries": entries,
		"admin":   showHidden,
	}
	if showHidden {
		resp["hiddenPaths"] = a.HiddenFolders()
	}
	writeJSON(w, resp)
}

// filterHidden 从目录条目里去掉已隐藏的文件夹。
func filterHidden(a *App, dir string, entries []files.Entry) []files.Entry {
	list := a.HiddenFolders()
	if len(list) == 0 {
		return entries
	}
	hidden := make(map[string]bool, len(list))
	for _, h := range list {
		hidden[h] = true
	}
	out := entries[:0]
	for _, e := range entries {
		if hidden[joinURL(dir, e.Name)] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// joinURL 拼接 URL 风格的相对路径（dir 以 "/" 开头）。
func joinURL(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

// parentURL 返回 URL 风格相对路径的父目录（"/a/b" → "/a"，"/a" → "/"）。
func parentURL(p string) string {
	p = "/" + strings.Trim(p, "/")
	if p == "/" {
		return "/"
	}
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}

func (a *App) handleFile(w http.ResponseWriter, r *http.Request) {
	files.ServeFile(w, r, a.RootDir(), r.URL.Query().Get("path"))
}

// handleThumb 为图片/视频生成并返回缩略图。浏览器按 ?v= 缓存，文件变化时 v 改变即失效。
func (a *App) handleThumb(w http.ResponseWriter, r *http.Request) {
	full, err := files.Resolve(a.RootDir(), r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	kind := files.TypeOf(filepath.Base(full))
	if kind != "image" && kind != "video" {
		http.Error(w, "not a media file", http.StatusBadRequest)
		return
	}
	res, err := thumbs.Generate(full, kind, thumbs.DefaultSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(res.Data)
}

// handleUpload 接收 multipart 文件上传并流式写入磁盘；HEIC/HEIF 先转 JPEG。
func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	dir, err := files.Resolve(a.RootDir(), r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		http.Error(w, "invalid dir", http.StatusBadRequest)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "bad multipart", http.StatusBadRequest)
		return
	}
	type upResult struct {
		Name  string `json:"name"`
		Saved string `json:"saved,omitempty"`
		Error string `json:"error,omitempty"`
	}
	var results []upResult
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "results": results, "error": err.Error()})
			return
		}
		name := part.FileName()
		if name == "" {
			continue
		}
		clean, err := files.SafeUploadName(name)
		if err != nil {
			results = append(results, upResult{Name: name, Error: err.Error()})
			continue
		}
		outName := clean
		var src io.Reader = part
		if files.IsHEIC(clean) {
			data, err := io.ReadAll(part)
			if err != nil {
				results = append(results, upResult{Name: name, Error: "read: " + err.Error()})
				continue
			}
			jpeg, err := thumbs.TranscodeToJPEG(data)
			if err != nil {
				results = append(results, upResult{Name: name, Error: "transcode: " + err.Error()})
				continue
			}
			outName = strings.TrimSuffix(clean, filepath.Ext(clean)) + ".jpg"
			src = bytes.NewReader(jpeg)
		}
		final := files.UniquePath(dir, outName)
		f, err := os.OpenFile(final, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			results = append(results, upResult{Name: name, Error: err.Error()})
			continue
		}
		_, copyErr := io.Copy(f, src)
		closeErr := f.Close()
		if copyErr != nil {
			results = append(results, upResult{Name: name, Error: copyErr.Error()})
			continue
		}
		if closeErr != nil {
			results = append(results, upResult{Name: name, Error: closeErr.Error()})
			continue
		}
		results = append(results, upResult{Name: name, Saved: filepath.Base(final)})
	}
	writeJSON(w, map[string]any{"ok": true, "results": results})
}

// handleDelete 把一组文件/目录移入回收站（可用 Ctrl+Z 撤销）。禁止删除根目录和回收站本身。
func (a *App) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	type delResult struct {
		Path  string `json:"path"`
		Error string `json:"error,omitempty"`
	}
	root := filepath.Clean(a.RootDir())
	trash := files.TrashDir(a.RootDir())
	var results []delResult
	var steps []undoStep
	for _, p := range req.Paths {
		full, err := files.Resolve(a.RootDir(), p)
		if err != nil {
			results = append(results, delResult{Path: p, Error: "forbidden"})
			continue
		}
		clean := filepath.Clean(full)
		if clean == root {
			results = append(results, delResult{Path: p, Error: "cannot delete root"})
			continue
		}
		if clean == filepath.Clean(trash) {
			results = append(results, delResult{Path: p, Error: "cannot delete trash"})
			continue
		}
		if err := os.MkdirAll(trash, 0o755); err != nil {
			results = append(results, delResult{Path: p, Error: err.Error()})
			continue
		}
		target := filepath.Join(trash, fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(full)))
		if err := os.Rename(full, target); err != nil {
			results = append(results, delResult{Path: p, Error: err.Error()})
			continue
		}
		steps = append(steps, undoStep{From: target, To: full})
		results = append(results, delResult{Path: p})
	}
	if len(steps) > 0 {
		a.pushUndo(UndoEntry{Kind: "delete", Desc: fmt.Sprintf("删除 %d 项", len(steps)), Steps: steps})
	}
	writeJSON(w, map[string]any{"ok": true, "results": results})
}

// handleMove 把一组文件/目录移动到目标目录；重名自动加 (1)、(2)…；禁止移动到自身内部。
// 移动成功后记录撤销信息，可用 Ctrl+Z 移回原处。
func (a *App) handleMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	dest, err := files.Resolve(a.RootDir(), req.Dest)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		http.Error(w, "invalid dest", http.StatusBadRequest)
		return
	}
	type moveResult struct {
		Path  string `json:"path"`
		Saved string `json:"saved,omitempty"`
		Error string `json:"error,omitempty"`
	}
	trash := files.TrashDir(a.RootDir())
	var results []moveResult
	var steps []undoStep
	for _, p := range req.Paths {
		full, err := files.Resolve(a.RootDir(), p)
		if err != nil {
			results = append(results, moveResult{Path: p, Error: "forbidden"})
			continue
		}
		if filepath.Clean(full) == filepath.Clean(trash) {
			results = append(results, moveResult{Path: p, Error: "cannot move trash"})
			continue
		}
		// 目标不能是源目录自身或其子孙目录
		if rel, rerr := filepath.Rel(full, dest); rerr == nil &&
			rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			results = append(results, moveResult{Path: p, Error: "cannot move into itself"})
			continue
		}
		base := filepath.Base(full)
		if filepath.Dir(full) == dest {
			results = append(results, moveResult{Path: p, Saved: base}) // 已在目标目录
			continue
		}
		target := files.UniquePath(dest, base)
		if err := os.Rename(full, target); err != nil {
			results = append(results, moveResult{Path: p, Error: err.Error()})
			continue
		}
		steps = append(steps, undoStep{From: target, To: full})
		results = append(results, moveResult{Path: p, Saved: filepath.Base(target)})
	}
	if len(steps) > 0 {
		a.pushUndo(UndoEntry{Kind: "move", Desc: fmt.Sprintf("移动 %d 项", len(steps)), Steps: steps})
	}
	writeJSON(w, map[string]any{"ok": true, "results": results})
}

// handleCopy 把一组文件/目录复制到目标目录；重名自动加 (1)、(2)…；禁止复制到自身内部。
func (a *App) handleCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	dest, err := files.Resolve(a.RootDir(), req.Dest)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		http.Error(w, "invalid dest", http.StatusBadRequest)
		return
	}
	type copyResult struct {
		Path  string `json:"path"`
		Saved string `json:"saved,omitempty"`
		Error string `json:"error,omitempty"`
	}
	root := filepath.Clean(a.RootDir())
	trash := files.TrashDir(a.RootDir())
	var results []copyResult
	for _, p := range req.Paths {
		full, err := files.Resolve(a.RootDir(), p)
		if err != nil {
			results = append(results, copyResult{Path: p, Error: "forbidden"})
			continue
		}
		clean := filepath.Clean(full)
		if clean == root {
			results = append(results, copyResult{Path: p, Error: "cannot copy root"})
			continue
		}
		if clean == filepath.Clean(trash) {
			results = append(results, copyResult{Path: p, Error: "cannot copy trash"})
			continue
		}
		// 目标不能是源目录自身或其子孙目录
		if rel, rerr := filepath.Rel(clean, dest); rerr == nil &&
			rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			results = append(results, copyResult{Path: p, Error: "cannot copy into itself"})
			continue
		}
		base := filepath.Base(clean)
		target := files.UniquePath(dest, base)
		if err := files.CopyTree(clean, target); err != nil {
			results = append(results, copyResult{Path: p, Error: err.Error()})
			continue
		}
		results = append(results, copyResult{Path: p, Saved: filepath.Base(target)})
	}
	writeJSON(w, map[string]any{"ok": true, "results": results})
}

// handleRename 重命名单个文件/文件夹（同目录内改名），重名自动加 (1)、(2)…
func (a *App) handleRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	full, err := files.Resolve(a.RootDir(), req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "路径无效"})
		return
	}
	root := filepath.Clean(a.RootDir())
	clean := filepath.Clean(full)
	if clean == root {
		writeJSON(w, map[string]any{"ok": false, "error": "不能重命名根目录"})
		return
	}
	if filepath.Base(clean) == files.TrashName {
		writeJSON(w, map[string]any{"ok": false, "error": "不能重命名回收站"})
		return
	}
	name, err := files.SafeUploadName(req.Name)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "名称无效"})
		return
	}
	if name == filepath.Base(clean) {
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	target := files.UniquePath(filepath.Dir(clean), name)
	if err := os.Rename(clean, target); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// 若重命名的是隐藏文件夹，同步更新配置里的隐藏路径
	if rel, rerr := files.URLPath(req.Path); rerr == nil {
		newRel := joinURL(parentURL(rel), filepath.Base(target))
		a.Update(func(c *config.Config) {
			for i, h := range c.HiddenFolders {
				if h == rel {
					c.HiddenFolders[i] = newRel
				}
			}
		})
	}
	writeJSON(w, map[string]any{"ok": true, "name": filepath.Base(target)})
}

// handleCreate 在当前目录下新建一个文件夹或空文件，重名自动加 (1)、(2)…
func (a *App) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"` // 目标目录
		Type string `json:"type"` // dir | file
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	dir, err := files.Resolve(a.RootDir(), req.Path)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		http.Error(w, "invalid dir", http.StatusBadRequest)
		return
	}
	name, err := files.SafeUploadName(req.Name)
	if err != nil {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	final := files.UniquePath(dir, name)
	if req.Type == "dir" {
		if err := os.Mkdir(final, 0o755); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	} else {
		f, err := os.OpenFile(final, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_ = f.Close()
	}
	writeJSON(w, map[string]any{"ok": true, "name": filepath.Base(final)})
}

// handleSearch 按名称递归搜索文件/目录（忽略大小写），返回匹配项及其所在目录。
// 访客账号看不到隐藏文件夹里的内容；回收站不参与搜索。
func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, map[string]any{"ok": true, "results": []any{}})
		return
	}
	showHidden := a.isAdminAccount(r)
	root := a.RootDir()
	ql := strings.ToLower(q)

	type searchHit struct {
		Path    string `json:"path"`
		Name    string `json:"name"`
		Parent  string `json:"parent"`
		IsDir   bool   `json:"isDir"`
		Type    string `json:"type"`
		Size    int64  `json:"size"`
		ModTime int64  `json:"modTime"`
	}

	const limit = 200
	results := make([]searchHit, 0, 32)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过无权限/损坏项
		}
		if len(results) >= limit {
			return fs.SkipAll
		}
		if p == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() && (name == files.TrashName || name == files.ChatDirName) {
			return filepath.SkipDir
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		urlPath := "/" + filepath.ToSlash(rel)
		if !showHidden && a.IsHiddenPath(urlPath) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.Contains(strings.ToLower(name), ql) {
			return nil
		}
		hit := searchHit{
			Path:   urlPath,
			Name:   name,
			Parent: parentURL(urlPath),
			IsDir:  d.IsDir(),
			Type:   "other",
		}
		if d.IsDir() {
			hit.Type = "dir"
		} else {
			hit.Type = files.TypeOf(name)
		}
		if fi, ferr := d.Info(); ferr == nil {
			hit.Size = fi.Size()
			hit.ModTime = fi.ModTime().Unix()
		}
		results = append(results, hit)
		return nil
	})
	writeJSON(w, map[string]any{"ok": true, "results": results})
}

// handleSave 覆盖写回文本文件内容（移动端编辑文本文件用）。
func (a *App) handleSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	full, err := files.Resolve(a.RootDir(), req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "路径无效"})
		return
	}
	clean := filepath.Clean(full)
	if clean == filepath.Clean(a.RootDir()) {
		writeJSON(w, map[string]any{"ok": false, "error": "不能编辑根目录"})
		return
	}
	if filepath.Base(clean) == files.TrashName {
		writeJSON(w, map[string]any{"ok": false, "error": "不能编辑回收站"})
		return
	}
	if !files.IsEditableText(filepath.Base(clean)) {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持编辑文本文件"})
		return
	}
	st, err := os.Stat(clean)
	if err != nil || st.IsDir() {
		writeJSON(w, map[string]any{"ok": false, "error": "文件不存在"})
		return
	}
	const maxSize = 5 << 20 // 5 MiB
	if len(req.Content) > maxSize {
		writeJSON(w, map[string]any{"ok": false, "error": "内容过大"})
		return
	}
	if err := os.WriteFile(clean, []byte(req.Content), 0o644); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleSleep 让 PC 进入睡眠（仅管理员账号可用）。先回包再触发，避免响应在睡眠前丢失。
func (a *App) handleSleep(w http.ResponseWriter, r *http.Request) {
	if !a.isAdminAccount(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
	go func() {
		time.Sleep(300 * time.Millisecond)
		// SetSuspendState(Hibernate=0, Force=1, DisableWakeEvent=0)：进入睡眠。
		_ = exec.Command("rundll32", "powrprof.dll,SetSuspendState", "0", "1", "0").Start()
	}()
}

// handleUndo 撤销最近一次删除/移动操作（Ctrl+Z）。
func (a *App) handleUndo(w http.ResponseWriter, r *http.Request) {
	e := a.popUndo()
	if e == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "没有可撤销的操作"})
		return
	}
	type undoResult struct {
		Path  string `json:"path"`
		Error string `json:"error,omitempty"`
	}
	var results []undoResult
	for _, s := range e.Steps {
		if err := os.Rename(s.From, s.To); err != nil {
			results = append(results, undoResult{Path: s.To, Error: err.Error()})
		} else {
			results = append(results, undoResult{Path: s.To})
		}
	}
	writeJSON(w, map[string]any{"ok": true, "desc": e.Desc, "results": results})
}

// handleEmptyTrash 永久清空回收站，并移除已失效的删除撤销记录。
func (a *App) handleEmptyTrash(w http.ResponseWriter, r *http.Request) {
	trash := files.TrashDir(a.RootDir())
	if err := os.RemoveAll(trash); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	a.removeUndoKind("delete")
	writeJSON(w, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.RequestURI(), time.Since(start))
	})
}
