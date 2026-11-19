package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nas/internal/files"
)

// removeChatFile 删除消息附件（file 为 /.nas_chat/files/<name> 形式的相对 URL）。
func removeChatFile(dir, file string) {
	if file == "" {
		return
	}
	base := filepath.Base(filepath.FromSlash(strings.TrimPrefix(file, "/")))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return
	}
	_ = os.Remove(filepath.Join(dir, base))
}

// 未引用附件的分组名：磁盘上有文件，但没有任何消息指向它。
const orphanPeerLabel = "（未引用）"

type chatUsageGroup struct {
	Peer  string `json:"peer"`
	Count int    `json:"count"`
	Bytes int64  `json:"bytes"`
}

// handleChatStats 统计对话附件占用：总量 + 按对话对象分组（管理面板用）。
// 以磁盘上的实际文件为准，未被任何消息引用的文件归到「未引用」。
func (a *App) handleChatStats(w http.ResponseWriter, r *http.Request) {
	me, isPC := a.chatIdentity(r)
	if me == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !isPC {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	root := a.RootDir()
	if root == "" {
		writeJSON(w, map[string]any{"ok": true, "count": 0, "bytes": 0, "groups": []chatUsageGroup{}})
		return
	}
	dir := files.ChatFilesDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, map[string]any{"ok": true, "count": 0, "bytes": 0, "groups": []chatUsageGroup{}})
		return
	}
	owners := a.chat().AttachmentOwners(a.deviceName())

	byPeer := map[string]*chatUsageGroup{}
	var totalCount int
	var totalBytes int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		peer, ok := owners[e.Name()]
		if !ok || peer == "" {
			peer = orphanPeerLabel
		}
		g := byPeer[peer]
		if g == nil {
			g = &chatUsageGroup{Peer: peer}
			byPeer[peer] = g
		}
		g.Count++
		g.Bytes += info.Size()
		totalCount++
		totalBytes += info.Size()
	}

	groups := make([]chatUsageGroup, 0, len(byPeer))
	for _, g := range byPeer {
		groups = append(groups, *g)
	}
	// 占用大的排在前面
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Bytes != groups[j].Bytes {
			return groups[i].Bytes > groups[j].Bytes
		}
		return groups[i].Peer < groups[j].Peer
	})

	writeJSON(w, map[string]any{
		"ok":     true,
		"count":  totalCount,
		"bytes":  totalBytes,
		"groups": groups,
	})
}

// handleChatClearAll 清空全部对话附件：删除磁盘上所有附件，
// 以及引用它们的消息（纯文字消息保留）。管理面板用。
func (a *App) handleChatClearAll(w http.ResponseWriter, r *http.Request) {
	me, isPC := a.chatIdentity(r)
	if me == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !isPC {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	root := a.RootDir()
	if root == "" {
		writeJSON(w, map[string]any{"ok": true, "messages": 0, "files": 0, "freed": 0})
		return
	}
	removed := a.chat().DeleteAttachments()

	dir := files.ChatFilesDir(root)
	var count int
	var freed int64
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			full := filepath.Join(dir, e.Name())
			var size int64
			if info, err := e.Info(); err == nil {
				size = info.Size()
			}
			if err := os.Remove(full); err == nil {
				count++
				freed += size
			}
		}
	}
	writeJSON(w, map[string]any{
		"ok":       true,
		"messages": len(removed),
		"files":    count,
		"freed":    freed,
	})
}
