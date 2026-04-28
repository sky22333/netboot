package observability

import (
	"log/slog"
	"sync"
	"time"
)

type Event struct {
	ID      uint64 `json:"id"`
	Time    string `json:"time"`
	Level   string `json:"level"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	recent      []Event
	maxRecent   int
	nextID      uint64
}

func NewHub() *Hub {
	return &Hub{subscribers: map[chan Event]struct{}{}, maxRecent: 1000}
}

func (h *Hub) Publish(level, source, message string) {
	now := time.Now()
	switch level {
	case "error":
		slog.Error(message, "source", source)
	case "warning":
		slog.Warn(message, "source", source)
	default:
		slog.Info(message, "source", source)
	}
	h.mu.Lock()
	h.nextID++
	event := Event{ID: h.nextID, Time: now.Format(time.RFC3339Nano), Level: level, Source: source, Message: message}
	h.recent = append(h.recent, event)
	if len(h.recent) > h.maxRecent {
		h.recent = h.recent[len(h.recent)-h.maxRecent:]
	}
	for ch := range h.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
	h.mu.Unlock()
}

func (h *Hub) Subscribe() (chan Event, func()) {
	ch := make(chan Event, 32)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subscribers, ch)
		close(ch)
		h.mu.Unlock()
	}
}

func (h *Hub) Recent() []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Event, len(h.recent))
	copy(out, h.recent)
	return out
}
