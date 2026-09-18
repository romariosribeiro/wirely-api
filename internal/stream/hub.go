package stream

import (
	"sync"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan engine.Event]struct{}
}

func New() *Hub { return &Hub{subscribers: make(map[string]map[chan engine.Event]struct{})} }

func (h *Hub) Publish(event engine.Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range h.subscribers[event.InstanceID] {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (h *Hub) Subscribe(instanceID string) (<-chan engine.Event, func()) {
	channel := make(chan engine.Event, 64)
	h.mu.Lock()
	if h.subscribers[instanceID] == nil {
		h.subscribers[instanceID] = make(map[chan engine.Event]struct{})
	}
	h.subscribers[instanceID][channel] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subscribers[instanceID], channel)
			if len(h.subscribers[instanceID]) == 0 {
				delete(h.subscribers, instanceID)
			}
			h.mu.Unlock()
			close(channel)
		})
	}
	return channel, cancel
}
