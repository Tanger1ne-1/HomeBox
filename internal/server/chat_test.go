package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nas/internal/chat"
	"nas/internal/config"
)

// newTestApp 建一个临时根目录的 App，并登记手机端会话 "tok" 与管理面板会话 "atk"。
func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	app := NewApp(&config.Config{RootDir: dir, Port: 8377}, filepath.Join(dir, "nasd.json"))
	app.sessMu.Lock()
	app.sessions["tok"] = &Session{
		Username: "u", Role: config.RoleGuest, Kind: sessionMobile,
		Expires: time.Now().Add(time.Hour), LastSeen: time.Now(),
	}
	app.sessions["atk"] = &Session{
		Username: "admin", Role: config.RoleAdmin, Kind: sessionAdmin,
		Expires: time.Now().Add(time.Hour), LastSeen: time.Now(),
	}
	app.sessMu.Unlock()
	return app, dir
}

func doJSONAs(t *testing.T, h http.HandlerFunc, token, method, target string, body map[string]any) (int, map[string]any) {
	t.Helper()
	var r *http.Request
	if body != nil {
		buf, _ := json.Marshal(body)
		r = httptest.NewRequest(method, target, bytes.NewReader(buf))
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	w := httptest.NewRecorder()
	h(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestChatMessagesPaging(t *testing.T) {
	app, _ := newTestApp(t)
	store := app.chat()
	for i := 0; i < 5; i++ {
		if _, err := store.Add(chat.Message{From: "u", To: app.deviceName(), Type: "text"}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	peer := app.deviceName()

	code, out := doJSONAs(t, app.handleChatMessages, "tok", "GET",
		"/api/chat/messages?peer="+peer+"&limit=2", nil)
	if code != http.StatusOK || out["hasMore"] != true {
		t.Fatalf("code=%d out=%v", code, out)
	}
	msgs, _ := out["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("第一页 %d 条，期望 2", len(msgs))
	}
	first := msgs[0].(map[string]any)["seq"].(float64)
	if first != 4 {
		t.Fatalf("第一页首条 seq=%v，期望 4", first)
	}
}

func TestChatStatsAndClearAll(t *testing.T) {
	app, dir := newTestApp(t)
	store := app.chat()
	filesDir := filepath.Join(dir, ".nas_chat", "files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// a.jpg 被消息引用，orphan.jpg 没有
	if err := os.WriteFile(filepath.Join(filesDir, "a.jpg"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "orphan.jpg"), []byte("xy"), 0o644); err != nil {
		t.Fatal(err)
	}
	peer := app.deviceName()
	_, _ = store.Add(chat.Message{From: "u", To: peer, Type: "image", File: "/.nas_chat/files/a.jpg"})
	_, _ = store.Add(chat.Message{From: "u", To: peer, Type: "text", Text: "hi"})

	code, out := doJSONAs(t, app.handleChatStats, "atk", "GET", "/api/chat/stats", nil)
	if code != http.StatusOK {
		t.Fatalf("stats code=%d", code)
	}
	if out["count"].(float64) != 2 || out["bytes"].(float64) != 5 {
		t.Fatalf("stats = %v", out)
	}
	groups, _ := out["groups"].([]any)
	if len(groups) != 2 {
		t.Fatalf("分组 %d 个，期望 2（u / 未引用）", len(groups))
	}
	var uGroup map[string]any
	for _, g := range groups {
		m := g.(map[string]any)
		if m["peer"] == "u" {
			uGroup = m
		}
	}
	if uGroup == nil || uGroup["bytes"].(float64) != 3 || uGroup["count"].(float64) != 1 {
		t.Fatalf("u 分组 = %v", uGroup)
	}

	// 清空：删 2 个文件、1 条带附件的消息，文字消息保留
	code, out = doJSONAs(t, app.handleChatClearAll, "atk", "POST", "/api/chat/clearall", map[string]any{})
	if code != http.StatusOK || out["messages"].(float64) != 1 || out["files"].(float64) != 2 {
		t.Fatalf("clearall code=%d out=%v", code, out)
	}
	if left := store.Messages("u", peer, 0); len(left) != 1 || left[0].Type != "text" {
		t.Fatalf("剩余消息 = %v", left)
	}
	entries, _ := os.ReadDir(filesDir)
	if len(entries) != 0 {
		t.Fatalf("附件目录仍有 %d 个文件", len(entries))
	}
}

// TestChatStatsForbiddenForMobile 确认手机端会话不能调用管理接口。
func TestChatStatsForbiddenForMobile(t *testing.T) {
	app, _ := newTestApp(t)
	code, _ := doJSONAs(t, app.handleChatStats, "tok", "GET", "/api/chat/stats", nil)
	if code != http.StatusForbidden {
		t.Fatalf("code=%d，期望 403", code)
	}
}
