package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nas/internal/chat"
	"nas/internal/files"
	"nas/internal/thumbs"
)

// deviceName 返回对话里 PC 一方的显示名称（配置的设备名，留空用主机名）。
func (a *App) deviceName() string {
	if n := strings.TrimSpace(a.Config().DeviceName); n != "" {
		return n
	}
	if h, err := os.Hostname(); err == nil && strings.TrimSpace(h) != "" {
		return strings.TrimSpace(h)
	}
	return "HomeBox"
}

// chatIdentity 返回当前会话在对话里的身份：me 为发送方标识，isPC 表示是管理面板（PC）一侧。
func (a *App) chatIdentity(r *http.Request) (me string, isPC bool) {
	s := a.getSession(tokenFromRequest(r))
	if s == nil {
		return "", false
	}
	if s.Kind == sessionAdmin {
		return a.deviceName(), true
	}
	return s.Username, false
}

// validChatPeer 校验消息接收方：PC 只能发给账号，移动端只能发给设备名。
func (a *App) validChatPeer(r *http.Request, to string, isPC bool) bool {
	to = strings.TrimSpace(to)
	if to == "" {
		return false
	}
	if isPC {
		return a.Config().Find(to) != nil
	}
	return to == a.deviceName()
}

// handleChatState 返回对话所需的身份信息（设备名 + 当前用户）。
func (a *App) handleChatState(w http.ResponseWriter, r *http.Request) {
	me, isPC := a.chatIdentity(r)
	if me == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{
		"ok":         true,
		"deviceName": a.deviceName(),
		"me":         me,
		"isPC":       isPC,
	})
}

// handleChatMessages 增量返回当前用户与 peer 之间的新消息（after 为已见最大 seq）。
func (a *App) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	me, _ := a.chatIdentity(r)
	if me == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	peer := strings.TrimSpace(r.URL.Query().Get("peer"))
	if peer == "" {
		writeJSON(w, map[string]any{"ok": true, "messages": []chat.Message{}, "me": me, "deviceName": a.deviceName()})
		return
	}
	var after int64
	if v, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64); err == nil {
		after = v
	}
	writeJSON(w, map[string]any{
		"ok":         true,
		"messages":   a.chat().Messages(me, peer, after),
		"me":         me,
		"deviceName": a.deviceName(),
	})
}

// handleChatText 发送一条文本消息。
func (a *App) handleChatText(w http.ResponseWriter, r *http.Request) {
	me, isPC := a.chatIdentity(r)
	if me == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "消息不能为空"})
		return
	}
	if !a.validChatPeer(r, req.To, isPC) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	m, err := a.chat().Add(chat.Message{From: me, To: strings.TrimSpace(req.To), Type: "text", Text: text})
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": m})
}

// handleChatUpload 接收发送到对话里的图片/视频/文件，落盘并记为一条消息。
func (a *App) handleChatUpload(w http.ResponseWriter, r *http.Request) {
	me, isPC := a.chatIdentity(r)
	if me == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if !a.validChatPeer(r, to, isPC) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "bad multipart", http.StatusBadRequest)
		return
	}
	dir := files.ChatFilesDir(a.RootDir())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	added := make([]chat.Message, 0, 4)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := part.FileName()
		if name == "" {
			continue
		}
		clean, err := files.SafeUploadName(name)
		if err != nil {
			continue
		}
		outName := clean
		typ := chatTypeOf(clean)
		var src io.Reader = part
		if typ == "image" && files.IsHEIC(clean) {
			data, err := io.ReadAll(part)
			if err != nil {
				continue
			}
			jpeg, err := thumbs.TranscodeToJPEG(data)
			if err != nil {
				continue
			}
			outName = strings.TrimSuffix(clean, filepath.Ext(clean)) + ".jpg"
			src = bytes.NewReader(jpeg)
		}
		final := files.UniquePath(dir, outName)
		f, err := os.OpenFile(final, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			continue
		}
		size, copyErr := io.Copy(f, src)
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil {
			continue
		}
		relURL := "/" + filepath.ToSlash(filepath.Join(files.ChatDirName, "files", filepath.Base(final)))
		m, err := a.chat().Add(chat.Message{
			From:     me,
			To:       to,
			Type:     typ,
			File:     relURL,
			FileName: clean,
			Size:     size,
		})
		if err != nil {
			continue
		}
		added = append(added, m)
	}
	writeJSON(w, map[string]any{"ok": true, "messages": added})
}

// chatTypeOf 把附件归为 image / video / file 三类（对话消息用）。
func chatTypeOf(name string) string {
	switch files.TypeOf(name) {
	case "image":
		return "image"
	case "video":
		return "video"
	default:
		return "file"
	}
}

// handleChatQuery 按时间范围返回当前用户与 peer 之间的消息（管理面板用）。
func (a *App) handleChatQuery(w http.ResponseWriter, r *http.Request) {
	me, isPC := a.chatIdentity(r)
	if me == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !isPC {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	peer := strings.TrimSpace(r.URL.Query().Get("peer"))
	if !a.validChatPeer(r, peer, true) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, err := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if err != nil || to <= 0 {
		to = 1 << 62
	}
	writeJSON(w, map[string]any{"ok": true, "messages": a.chat().Between(me, peer, from, to)})
}

// handleChatClear 清空当前用户与 peer 之间的全部消息及其附件（管理面板用）。
func (a *App) handleChatClear(w http.ResponseWriter, r *http.Request) {
	me, isPC := a.chatIdentity(r)
	if me == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !isPC {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	peer := strings.TrimSpace(req.To)
	if !a.validChatPeer(r, peer, true) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	removed := a.chat().Clear(me, peer)
	dir := files.ChatFilesDir(a.RootDir())
	for _, m := range removed {
		if m.File == "" {
			continue
		}
		base := filepath.Base(filepath.FromSlash(strings.TrimPrefix(m.File, "/")))
		if base != "" && base != "." && base != string(filepath.Separator) {
			_ = os.Remove(filepath.Join(dir, base))
		}
	}
	writeJSON(w, map[string]any{"ok": true, "removed": len(removed)})
}
