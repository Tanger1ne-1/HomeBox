// Package chat 提供「文件传输助手」式的一对一会话持久化。
// 对话发生在 PC（以设备名标识）与某个移动端用户（以用户名标识）之间；
// 消息连同附件一起保存在 HomeBox 根目录下的 .nas_chat 目录里，随服务重启保留。
package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
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
}

// Store 在内存里维护消息列表并落盘 JSON，线程安全。
type Store struct {
	mu   sync.Mutex
	path string
	seq  int64
	msgs []Message
}

// Open 从 messagesPath 加载对话历史；文件不存在或损坏则从空开始。
func Open(messagesPath string) *Store {
	s := &Store{path: messagesPath}
	if data, err := os.ReadFile(messagesPath); err == nil {
		var msgs []Message
		if json.Unmarshal(data, &msgs) == nil {
			s.msgs = msgs
		}
	}
	for _, m := range s.msgs {
		if m.Seq > s.seq {
			s.seq = m.Seq
		}
	}
	return s
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
	_ = s.persist()
	return removed
}

// Add 追加一条消息并落盘，返回补全 seq/time 后的完整消息。
func (s *Store) Add(m Message) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	m.Seq = s.seq
	if m.Time == 0 {
		m.Time = time.Now().Unix()
	}
	s.msgs = append(s.msgs, m)
	if err := s.persist(); err != nil {
		return Message{}, err
	}
	return m, nil
}

// persist 把全部消息写回 JSON 文件。
func (s *Store) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s.msgs)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
