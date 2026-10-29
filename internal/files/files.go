package files

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrOutsideRoot 表示请求的路径越出了 HomeBox 根目录。
var ErrOutsideRoot = errors.New("path escapes root")

// Entry 是目录中的一项（文件或子目录）。
type Entry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"` // Unix 秒
	Type    string `json:"type"`    // dir/image/video/audio/text/other
}

// cleanPath 把 URL 风格的相对路径规范化为安全的分段列表。
// 禁止 ".."、盘符、反斜杠等越界片段。
func cleanPath(p string) ([]string, error) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil, nil
	}
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			// 忽略空段和当前目录
		case "..":
			return nil, ErrOutsideRoot
		default:
			if strings.ContainsAny(part, `\:*?"<>|`) {
				return nil, ErrOutsideRoot
			}
			out = append(out, part)
		}
	}
	return out, nil
}

// Resolve 把 URL 相对路径解析为根目录下的绝对路径，保证不越界。
func Resolve(root, p string) (string, error) {
	segs, err := cleanPath(p)
	if err != nil {
		return "", err
	}
	if len(segs) == 0 {
		return root, nil
	}
	full := filepath.Join(append([]string{root}, segs...)...)
	// 双重校验：确保结果仍在 root 之内
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrOutsideRoot
	}
	return full, nil
}

// URLPath 返回规范化的 URL 风格路径（以 "/" 开头）。
func URLPath(p string) (string, error) {
	segs, err := cleanPath(p)
	if err != nil {
		return "", err
	}
	if len(segs) == 0 {
		return "/", nil
	}
	return "/" + strings.Join(segs, "/"), nil
}

// List 返回目录内容，目录项排前面，其余按名称排序。
func List(root, p string) (string, []Entry, error) {
	dir, err := Resolve(root, p)
	if err != nil {
		return "", nil, err
	}
	st, err := os.Stat(dir)
	if err != nil {
		return "", nil, err
	}
	if !st.IsDir() {
		return "", nil, errors.New("not a directory")
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, err
	}
	entries := make([]Entry, 0, len(des))
	for _, de := range des {
		if de.IsDir() && (de.Name() == TrashName || de.Name() == ChatDirName) {
			continue // 回收站/对话数据不显示在文件列表里，由专门入口访问
		}
		fi, err := de.Info()
		if err != nil {
			continue
		}
		e := Entry{
			Name:    de.Name(),
			IsDir:   de.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime().Unix(),
			Type:    "other",
		}
		if de.IsDir() {
			e.Type = "dir"
		} else {
			e.Type = detectType(de.Name())
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		// 文件按修改时间倒序（新照片在前），时间相同再按名称
		if !entries[i].IsDir && entries[i].ModTime != entries[j].ModTime {
			return entries[i].ModTime > entries[j].ModTime
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	canon, err := URLPath(p)
	if err != nil {
		return "", nil, err
	}
	return canon, entries, nil
}

// ServeFile 以流式方式输出文件，支持 HTTP Range（视频拖动/断点续传的基础）。
func ServeFile(w http.ResponseWriter, r *http.Request, root, p string) {
	full, err := Resolve(root, p)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if st.IsDir() {
		http.Error(w, "is a directory", http.StatusBadRequest)
		return
	}
	if ct := mimeType(full); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, filepath.Base(full), st.ModTime(), f)
}

// TypeOf 返回文件名对应的媒体类型（image/video/audio/text/dir/other）。
func TypeOf(name string) string { return detectType(name) }

// editableTextExts 是可安全用文本编辑器读写（不损坏二进制内容）的扩展名集合。
var editableTextExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".log": true, ".json": true,
	".xml": true, ".html": true, ".htm": true, ".csv": true, ".ini": true,
	".yml": true, ".yaml": true, ".js": true, ".css": true, ".py": true,
	".go": true, ".sh": true, ".bat": true, ".conf": true, ".properties": true,
}

// IsEditableText 判断文件名是否为可安全编辑的文本文件。
func IsEditableText(name string) bool {
	return editableTextExts[strings.ToLower(filepath.Ext(name))]
}

// SafeUploadName 清洗上传文件名，只保留纯文件名（去掉路径、盘符等危险片段）。
func SafeUploadName(name string) (string, error) {
	name = filepath.Base(name)
	if name == "." || name == ".." || name == "" {
		return "", errors.New("invalid filename")
	}
	if strings.ContainsAny(name, `\:*?"<>|`) {
		return "", errors.New("invalid filename")
	}
	return name, nil
}

// IsHEIC 判断文件名是否为 HEIC/HEIF 格式。
func IsHEIC(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".heic", ".heif":
		return true
	}
	return false
}

// UniquePath 在 dir 下为 name 生成不冲突的路径，重名时追加 (1)、(2)…
func UniquePath(dir, name string) string {
	full := filepath.Join(dir, name)
	if _, err := os.Lstat(full); err != nil {
		return full // 不存在，直接用
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Lstat(cand); err != nil {
			return cand
		}
	}
}

// CopyTree 递归复制 src 到 dst（src 可为文件或目录，dst 为目标路径，不自动重名）。
func CopyTree(src, dst string) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if st.IsDir() {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		des, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, de := range des {
			if err := CopyTree(filepath.Join(src, de.Name()), filepath.Join(dst, de.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func detectType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".heic", ".heif", ".svg":
		return "image"
	case ".mp4", ".mov", ".m4v", ".mkv", ".avi", ".webm", ".flv", ".wmv", ".ts":
		return "video"
	case ".mp3", ".m4a", ".wav", ".flac", ".aac", ".ogg", ".wma", ".opus":
		return "audio"
	case ".txt", ".md", ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".csv", ".json", ".xml", ".html", ".htm", ".log":
		return "text"
	default:
		return "other"
	}
}

func mimeType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".m4v":
		return "video/x-m4v"
	case ".mkv":
		return "video/x-matroska"
	case ".webm":
		return "video/webm"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	case ".svg":
		return "image/svg+xml"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".md":
		return "text/markdown; charset=utf-8"
	default:
		return ""
	}
}

// TrashName 是根目录下的回收站目录名（删除操作先移到这里，可用 Ctrl+Z 撤销）。
const TrashName = ".nas_trash"

// TrashDir 返回根目录下的回收站绝对路径。
func TrashDir(root string) string { return filepath.Join(root, TrashName) }

// ChatDirName 是根目录下存放对话记录与附件的系统目录名（文件列表/搜索里隐藏）。
const ChatDirName = ".nas_chat"

// ChatDir 返回根目录下的对话数据目录绝对路径。
func ChatDir(root string) string { return filepath.Join(root, ChatDirName) }

// ChatFilesDir 返回根目录下存放对话附件的目录绝对路径。
func ChatFilesDir(root string) string { return filepath.Join(root, ChatDirName, "files") }
