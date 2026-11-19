// Package chat 提供「文件传输助手」式的一对一会话持久化。
// 对话发生在 PC（以设备名标识）与某个移动端用户（以用户名标识）之间；
// 消息连同附件一起保存在 HomeBox 根目录下的 .nas_chat 目录里，随服务重启保留。
//
// 落盘格式是 NDJSON（每行一条消息，追加写入）：新增消息只追加一行，
// 不必把整个历史重写一遍，几千条消息时也不会越传越慢。
package chat

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Message 是一条对话消息。
type Message struct {
	Seq      int64  `json:"seq"`                // 单调递增序号（客户端据此增量拉取）
	From     string `json:"from"`               // 发送方：设备名或用户名
	To       string `json:"to"`                 // 接收方：用户名或设备名
	Type     string `json:"type"`               // text | image | video | file
	Text     string `json:"text,omitempty"`     // 文本内容
	File     string `json:"file,omitempty"`     // 附件相对 URL 路径（/…）
	FileName string `json:"fileName,omitempty"` // 附件原始文件名
	Size     int64  `json:"size,omitempty"`     // 附件字节数
	Time     int64  `json:"time"`               // Unix 秒
	CID      string `json:"cid,omitempty"`      // 客户端幂等标识：重传时据此去重
}

// Store 在内存里维护消息列表并落盘 NDJSON，线程安全。
type Store struct {
	mu   sync.Mutex
	path string // NDJSON 文件路径
	seq  int64
	msgs []Message
	subs map[chan struct{}]struct{} // 新消息通知（SSE 用）
}

// Open 加载对话历史。优先读 NDJSON；若只有旧版的 messages.json，
// 则读入后迁移为 NDJSON，旧文件改名为 .bak 备份。
func Open(messagesPath string) *Store {
	nd := messagesPath
	if !strings.HasSuffix(nd, ".ndjson") {
		nd = strings.TrimSuffix(nd, ".json") + ".ndjson"
	}
	s := &Store{path: nd, subs: map[chan struct{}]struct{}{}}

	if _, err := os.Stat(nd); err == nil {
		s.msgs = readNDJSON(nd)
	} else if nd != messagesPath {
		if data, err := os.ReadFile(messagesPath); err == nil {
			var legacy []Message
			if json.Unmarshal(data, &legacy) == nil && len(legacy) > 0 {
				s.msgs = legacy
				if err := s.compact(); err == nil {
					_ = os.Rename(messagesPath, messagesPath+".bak")
				}
			}
		}
	}

	for _, m := range s.msgs {
		if m.Seq > s.seq {
			s.seq = m.Seq
		}
	}
	return s
}

// readNDJSON 逐行解析；损坏的行（例如写到一半掉电）直接跳过，不影响其余消息。
func readNDJSON(path string) []Message {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := make([]Message, 0, 256)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var m Message
		if json.Unmarshal(line, &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

// Messages 返回 me 与 peer 之间 seq 大于 after 的消息（按 seq 升序）。
func (s *Store) Messages(me, peer string, after int64) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, 0, 8)
	for _, m := range s.msgs {
		if m.Seq <= after {
			continue
		}
		if (m.From == me && m.To == peer) || (m.From == peer && m.To == me) {
			out = append(out, m)
		}
	}
	return out
}

// Recent 返回 me 与 peer 之间 seq 小于 before 的最后 limit 条消息（before<=0 表示不限），
// 按 seq 升序返回；hasMore 表示更早的消息还没取完。
func (s *Store) Recent(me, peer string, before int64, limit int) ([]Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 200
	}
	out := make([]Message, 0, limit+1)
	for i := len(s.msgs) - 1; i >= 0 && len(out) <= limit; i-- {
		m := s.msgs[i]
		if before > 0 && m.Seq >= before {
			continue
		}
		if (m.From == me && m.To == peer) || (m.From == peer && m.To == me) {
			out = append(out, m)
		}
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, hasMore
}

// Between 返回 me 与 peer 之间时间落在 [from,to] 内的消息（按 seq 升序）。
func (s *Store) Between(me, peer string, from, to int64) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, 0, 8)
	for _, m := range s.msgs {
		if m.Time < from || m.Time > to {
			continue
		}
		if (m.From == me && m.To == peer) || (m.From == peer && m.To == me) {
			out = append(out, m)
		}
	}
	return out
}

// Clear 删除 me 与 peer 之间的全部消息并落盘，返回被删除的消息。
func (s *Store) Clear(me, peer string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept, removed []Message
	for _, m := range s.msgs {
		if (m.From == me && m.To == peer) || (m.From == peer && m.To == me) {
			removed = append(removed, m)
		} else {
			kept = append(kept, m)
		}
	}
	s.msgs = kept
	_ = s.compact()
	return removed
}

// Delete 删除指定 seq 且涉及 me 的消息并落盘，返回被删除的消息（未找到返回 false）。
func (s *Store) Delete(me string, seq int64) (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.msgs {
		if m.Seq == seq && (m.From == me || m.To == me) {
			s.msgs = append(s.msgs[:i], s.msgs[i+1:]...)
			_ = s.compact()
			return m, true
		}
	}
	return Message{}, false
}

// DeleteAttachments 删除所有带附件的消息并落盘，返回被删除的消息（纯文字消息保留）。
func (s *Store) DeleteAttachments() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept, removed []Message
	for _, m := range s.msgs {
		if m.File != "" {
			removed = append(removed, m)
		} else {
			kept = append(kept, m)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	s.msgs = kept
	_ = s.compact()
	return removed
}

// AttachmentOwners 返回附件文件名 → 对话对象（self 之外的另一方）。
// 没有消息引用的文件名不会出现在结果里。
func (s *Store) AttachmentOwners(self string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.msgs))
	for _, m := range s.msgs {
		if m.File == "" {
			continue
		}
		peer := m.From
		if m.From == self {
			peer = m.To
		}
		out[filepath.Base(filepath.FromSlash(m.File))] = peer
	}
	return out
}

// Add 追加一条消息并落盘，返回补全 seq/time 后的完整消息。
// 若 m.CID 非空且同一发送方已有相同 CID 的消息，直接返回旧消息（重传幂等）。
func (s *Store) Add(m Message) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.CID != "" {
		for _, old := range s.msgs {
			if old.CID == m.CID && old.From == m.From {
				return old, nil
			}
		}
	}
	s.seq++
	m.Seq = s.seq
	if m.Time == 0 {
		m.Time = time.Now().Unix()
	}
	s.msgs = append(s.msgs, m)
	s.notifyLocked()
	if err := s.appendLine(m); err != nil {
		return Message{}, err
	}
	return m, nil
}

// Subscribe 返回一个「有新消息」的通知通道，取消函数用于退订（SSE 用）。
func (s *Store) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

// notifyLocked 唤醒所有订阅者；调用方需持有 s.mu。
func (s *Store) notifyLocked() {
	for ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default: // 已经有未读通知，跳过
		}
	}
}

// appendLine 把一条消息追加到 NDJSON 末尾（O(1)，不重写历史）。
func (s *Store) appendLine(m Message) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// compact 把内存中的全部消息重写为 NDJSON（删除/清空/迁移后调用）。
// 先写临时文件再改名，避免写到一半崩溃把整个对话写坏。
func (s *Store) compact() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, m := range s.msgs {
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
