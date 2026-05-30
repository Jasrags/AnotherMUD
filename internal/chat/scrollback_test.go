package chat

import (
	"testing"
	"time"
)

func msg(text string, sec int) Message {
	return Message{
		PublishedAt: time.Date(2026, 5, 30, 12, 0, sec, 0, time.UTC),
		SenderID:    "p-1",
		SenderName:  "Alice",
		Text:        text,
	}
}

func TestScrollback_AppendAndTail(t *testing.T) {
	s := NewScrollback(5)
	s.Append(msg("a", 0))
	s.Append(msg("b", 1))
	s.Append(msg("c", 2))

	got := s.Tail(2)
	if len(got) != 2 || got[0].Text != "b" || got[1].Text != "c" {
		t.Errorf("Tail(2) = %v", textList(got))
	}
}

func TestScrollback_OldestEvicts(t *testing.T) {
	s := NewScrollback(2)
	s.Append(msg("a", 0))
	s.Append(msg("b", 1))
	s.Append(msg("c", 2)) // evict a

	got := s.Tail(5)
	if len(got) != 2 || got[0].Text != "b" || got[1].Text != "c" {
		t.Errorf("post-evict = %v", textList(got))
	}
	if s.Len() != 2 {
		t.Errorf("Len = %d, want 2", s.Len())
	}
}

func TestScrollback_TailMoreThanLen(t *testing.T) {
	s := NewScrollback(10)
	s.Append(msg("only", 0))
	got := s.Tail(50)
	if len(got) != 1 || got[0].Text != "only" {
		t.Errorf("Tail oversized = %v", textList(got))
	}
}

func TestScrollback_TailEmpty(t *testing.T) {
	s := NewScrollback(5)
	if got := s.Tail(10); got != nil {
		t.Errorf("empty Tail = %v, want nil", got)
	}
}

func TestScrollback_DefaultCapWhenNonPositive(t *testing.T) {
	s := NewScrollback(0)
	if s.Cap() != DefaultBufferCap {
		t.Errorf("Cap = %d, want default %d", s.Cap(), DefaultBufferCap)
	}
}

func textList(ms []Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Text
	}
	return out
}
