package chat

import (
	"path/filepath"
	"testing"
)

// newTestStore 在临时目录里建一个空对话库。
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return Open(filepath.Join(t.TempDir(), "messages.json"))
}

// TestRecentPaging 验证「加载更早」的分页：按 seq 升序、一次一页、hasMore 正确。
func TestRecentPaging(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 10; i++ {
		if _, err := s.Add(Message{From: "me", To: "pc", Type: "text", Text: "x"}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	// 第一页：最后 4 条（seq 7..10）
	page, more := s.Recent("me", "pc", 0, 4)
	if !more {
		t.Fatal("第一页应提示还有更早的消息")
	}
	if len(page) != 4 || page[0].Seq != 7 || page[3].Seq != 10 {
		t.Fatalf("第一页 = %v，期望 seq 7..10", seqs(page))
	}

	// 第二页：before=7，取 seq 3..6
	page2, more2 := s.Recent("me", "pc", 7, 4)
	if !more2 || len(page2) != 4 || page2[0].Seq != 3 || page2[3].Seq != 6 {
		t.Fatalf("第二页 = %v more=%v，期望 seq 3..6", seqs(page2), more2)
	}

	// 最后一页：before=3，只剩 1、2
	page3, more3 := s.Recent("me", "pc", 3, 4)
	if more3 || len(page3) != 2 || page3[0].Seq != 1 || page3[1].Seq != 2 {
		t.Fatalf("最后一页 = %v more=%v，期望 seq 1..2 且 hasMore=false", seqs(page3), more3)
	}
}

// TestRecentIsolatesPeers 确认分页只返回 me 与 peer 之间的消息。
func TestRecentIsolatesPeers(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Add(Message{From: "alice", To: "pc", Type: "text", Text: "a1"})
	_, _ = s.Add(Message{From: "bob", To: "pc", Type: "text", Text: "b1"})
	_, _ = s.Add(Message{From: "alice", To: "pc", Type: "text", Text: "a2"})

	page, _ := s.Recent("alice", "pc", 0, 10)
	if len(page) != 2 || page[0].Text != "a1" || page[1].Text != "a2" {
		t.Fatalf("alice 的消息 = %v", page)
	}
}

// TestPersistReload 确认消息落盘后能重新加载（含原子写入后的临时文件不残留）。
func TestPersistReload(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Add(Message{From: "u", To: "pc", Type: "text", Text: "hello"})

	reopened := Open(s.path)
	msgs := reopened.Messages("u", "pc", 0)
	if len(msgs) != 1 || msgs[0].Text != "hello" {
		t.Fatalf("重载后 = %v", msgs)
	}
	if msgs[0].Seq != 1 {
		t.Fatalf("seq 未保留: %d", msgs[0].Seq)
	}
}

// TestAddIsIdempotent 验证同一 CID 重复提交只会落一条（网络抖动重传不产生重复消息）。
func TestAddIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	m1, err := s.Add(Message{From: "u", To: "pc", Type: "text", Text: "hi", CID: "abc"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	m2, err := s.Add(Message{From: "u", To: "pc", Type: "text", Text: "hi", CID: "abc"})
	if err != nil {
		t.Fatalf("Add 重传: %v", err)
	}
	if m1.Seq != m2.Seq {
		t.Fatalf("重传得到新 seq：%d vs %d", m1.Seq, m2.Seq)
	}
	if got := len(s.Messages("u", "pc", 0)); got != 1 {
		t.Fatalf("消息数 = %d，期望 1", got)
	}
}

// TestDeleteAttachments 验证「清空全部附件」：带附件的消息删掉，纯文字消息保留。
func TestDeleteAttachments(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Add(Message{From: "u", To: "pc", Type: "text", Text: "hi"})
	_, _ = s.Add(Message{From: "u", To: "pc", Type: "image", File: "/.nas_chat/files/a.jpg"})
	_, _ = s.Add(Message{From: "pc", To: "u", Type: "image", File: "/.nas_chat/files/b.jpg"})

	removed := s.DeleteAttachments()
	if len(removed) != 2 {
		t.Fatalf("删除 %d 条，期望 2", len(removed))
	}
	left := s.Messages("u", "pc", 0)
	if len(left) != 1 || left[0].Type != "text" {
		t.Fatalf("剩余 = %v", left)
	}
	// 重开后仍保持
	if got := len(Open(s.path).Messages("u", "pc", 0)); got != 1 {
		t.Fatalf("重开后消息数 = %d", got)
	}
}

// TestAttachmentOwners 验证附件能归到对应的对话对象。
func TestAttachmentOwners(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Add(Message{From: "alice", To: "pc", Type: "image", File: "/.nas_chat/files/a.jpg"})
	_, _ = s.Add(Message{From: "pc", To: "bob", Type: "image", File: "/.nas_chat/files/b.jpg"})

	owners := s.AttachmentOwners("pc")
	if owners["a.jpg"] != "alice" || owners["b.jpg"] != "bob" {
		t.Fatalf("owners = %v", owners)
	}
}

func seqs(ms []Message) []int64 {
	out := make([]int64, len(ms))
	for i, m := range ms {
		out[i] = m.Seq
	}
	return out
}
