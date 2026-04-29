package realtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type Event struct {
	Name string
	Data any
}

type Hub struct {
	mu      sync.Mutex
	clients map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan Event]struct{})}
}

func (h *Hub) Subscribe() chan Event {
	ch := make(chan Event, 12)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	delete(h.clients, ch)
	close(ch)
	h.mu.Unlock()
}

func (h *Hub) Publish(name string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- Event{Name: name, Data: data}:
		default:
		}
	}
}

func WriteEvent(w http.ResponseWriter, event Event) error {
	b, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	if event.Name != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event.Name); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", b)
	return err
}
