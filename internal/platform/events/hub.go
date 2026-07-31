package events

import (
	"sync"
)

// Hub manages pub/sub topics and subscribers.
type Hub struct {
	mu     sync.RWMutex
	topics map[string][]chan any
}

// NewHub creates a new event hub.
func NewHub() *Hub {
	return &Hub{
		topics: make(map[string][]chan any),
	}
}

// Subscribe adds a subscriber to a topic and returns a channel.
// The caller is responsible for calling Unsubscribe when done.
func (h *Hub) Subscribe(topic string) <-chan any {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan any, 100)
	h.topics[topic] = append(h.topics[topic], ch)
	return ch
}

// Unsubscribe removes a subscriber from a topic.
func (h *Hub) Unsubscribe(topic string, ch <-chan any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	subs, ok := h.topics[topic]
	if !ok {
		return
	}

	for i, sub := range subs {
		if sub == ch {
			// Fast removal since order doesn't matter
			h.topics[topic][i] = h.topics[topic][len(subs)-1]
			h.topics[topic] = h.topics[topic][:len(subs)-1]
			close(sub)
			break
		}
	}

	if len(h.topics[topic]) == 0 {
		delete(h.topics, topic)
	}
}

// Publish broadcasts an event to all subscribers of a topic.
func (h *Hub) Publish(topic string, event any) {
	h.mu.RLock()
	subs, ok := h.topics[topic]
	if !ok {
		h.mu.RUnlock()
		return
	}

	// Copy the slice of channels to avoid holding the lock during send
	channels := make([]chan any, len(subs))
	copy(channels, subs)
	h.mu.RUnlock()

	for _, ch := range channels {
		select {
		case ch <- event:
		default:
			// If a subscriber is too slow and channel is full, drop the event
			// to avoid blocking the publisher.
		}
	}
}
