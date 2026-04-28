package observability

import (
	"log/slog"
	"sync"
	"time"
)

type Event struct {
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
}

func NewHub() *Hub {
	return &Hub{subscribers: map[chan Event]struct{}{}, maxRecent: 200}
}

func (h *Hub) Publish(level, source, message string) {
	event := Event{Time: time.Now().Format(time.RFC3339), Level: level, Source: source, Message: message}
	switch level {
	case "error":
		slog.Error(message, "source", source)
	case "warning":
		slog.Warn(message, "source", source)
	default:
		slog.Info(message, "source", source)
	}
	h.mu.Lock()
	h.recent = append([]Event{event}, h.recent...)
	if len(h.recent) > h.maxRecent {
		h.recent = h.recent[:h.maxRecent]
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
