package events

import (
	"strconv"
	"sync"
	"time"
)

// Event is a sequenced event that can be resumed with Last-Event-ID.
type Event struct {
	ID   uint64
	Data any
}

type topicState struct {
	legacy     []chan any
	sequenced  []chan Event
	history    []Event
	nextID     uint64
	lastActive time.Time
}

// Hub manages pub/sub topics and subscribers.
type Hub struct {
	mu     sync.RWMutex
	topics map[string]*topicState
}

// NewHub creates a new event hub.
func NewHub() *Hub {
	return &Hub{
		topics: make(map[string]*topicState),
	}
}

// Subscribe adds a subscriber to a topic and returns a channel.
// The caller is responsible for calling Unsubscribe when done.
func (h *Hub) Subscribe(topic string) <-chan any {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan any, 100)
	state := h.ensureTopic(topic)
	state.legacy = append(state.legacy, ch)
	return ch
}

// SubscribeFrom atomically subscribes and returns retained events newer than
// afterID. The retained window is bounded to the latest 512 events per topic.
func (h *Hub) SubscribeFrom(topic string, afterID uint64) (<-chan Event, []Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.ensureTopic(topic)
	ch := make(chan Event, 100)
	state.sequenced = append(state.sequenced, ch)
	replay := make([]Event, 0, len(state.history))
	for _, event := range state.history {
		if event.ID > afterID {
			replay = append(replay, event)
		}
	}
	return ch, replay
}

func (h *Hub) ensureTopic(topic string) *topicState {
	state := h.topics[topic]
	if state == nil {
		state = &topicState{lastActive: time.Now()}
		h.topics[topic] = state
	}
	state.lastActive = time.Now()
	return state
}

// Unsubscribe removes a subscriber from a topic.
func (h *Hub) Unsubscribe(topic string, ch <-chan any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	state, ok := h.topics[topic]
	if !ok {
		return
	}

	for i, sub := range state.legacy {
		if sub == ch {
			// Fast removal since order doesn't matter
			state.legacy[i] = state.legacy[len(state.legacy)-1]
			state.legacy = state.legacy[:len(state.legacy)-1]
			break
		}
	}

	state.lastActive = time.Now()
}

// UnsubscribeEvent removes a sequenced subscriber while retaining replay data.
func (h *Hub) UnsubscribeEvent(topic string, ch <-chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.topics[topic]
	if state == nil {
		return
	}
	for i, sub := range state.sequenced {
		if sub == ch {
			state.sequenced[i] = state.sequenced[len(state.sequenced)-1]
			state.sequenced = state.sequenced[:len(state.sequenced)-1]
			break
		}
	}
	state.lastActive = time.Now()
}

// Publish broadcasts an event to all subscribers of a topic.
func (h *Hub) Publish(topic string, event any) {
	h.mu.Lock()
	state := h.ensureTopic(topic)
	state.nextID++
	sequencedEvent := Event{ID: state.nextID, Data: event}
	state.history = append(state.history, sequencedEvent)
	if len(state.history) > 512 {
		state.history = append([]Event(nil), state.history[len(state.history)-512:]...)
	}
	channels := append([]chan any(nil), state.legacy...)
	sequenced := append([]chan Event(nil), state.sequenced...)
	// Opportunistically remove inactive topics without subscribers.
	for name, candidate := range h.topics {
		if name != topic && len(candidate.legacy) == 0 && len(candidate.sequenced) == 0 && time.Since(candidate.lastActive) > 24*time.Hour {
			delete(h.topics, name)
		}
	}
	h.mu.Unlock()

	for _, ch := range channels {
		select {
		case ch <- event:
		default:
			// If a subscriber is too slow and channel is full, drop the event
			// to avoid blocking the publisher.
		}
	}
	for _, ch := range sequenced {
		select {
		case ch <- sequencedEvent:
		default:
		}
	}
}

// ParseLastEventID accepts the SSE header/query representation.
func ParseLastEventID(value string) uint64 {
	id, _ := strconv.ParseUint(value, 10, 64)
	return id
}
