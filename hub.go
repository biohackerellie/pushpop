package pushpop

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// Message represents a message sent to clients.
type Message struct {
	Channel string      `json:"channel"`
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}

// Subscription represents a client subscription to a channel.
type Subscription struct {
	Client  *Client
	Channel string
}

// Hub maintains the set of active clients and broadcasts messages.
type Hub struct {
	clients    sync.Map
	broadcast  chan Message
	register   chan *Subscription
	unregister chan *Subscription
	channels   sync.Map
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan Message, 100),
		register:   make(chan *Subscription, 100),
		unregister: make(chan *Subscription, 100),
		channels:   sync.Map{},
		clients:    sync.Map{},
	}
}

// Run processes incoming events for the Hub.
func (h *Hub) Run() {
	for {
		select {
		case sub := <-h.register:
			h.addSubscription(sub)
		case sub := <-h.unregister:
			h.removeSubscription(sub)
		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

func (h *Hub) addSubscription(sub *Subscription) {
	val, _ := h.channels.LoadOrStore(sub.Channel, &sync.Map{})
	clients := val.(*sync.Map)
	clients.Store(sub.Client, true)
	sub.Client.channels.Store(sub.Channel, true)
}

func (h *Hub) removeSubscription(sub *Subscription) {
	val, ok := h.channels.Load(sub.Channel)
	if ok {
		clients := val.(*sync.Map)

		sub.Client.channels.Delete(sub.Channel)
		clients.Delete(sub.Client)
		count := 0

		clients.Range(func(_, _ interface{}) bool {
			count++
			return false
		})
		if count == 0 {
			h.channels.Delete(sub.Channel)
		}
	}
}

func (h *Hub) broadcastMessage(message Message) {
	val, ok := h.channels.Load(message.Channel)
	if ok {
		clients := val.(*sync.Map)
		clients.Range(func(key, _ interface{}) bool {
			client := key.(*Client)
			select {
			case client.send <- message:
			default:
				h.RemoveClient(client)
			}
			return true
		})
	}
}

// Trigger sends a message to all clients subscribed to a channel.
func (h *Hub) Trigger(message Message) {
	h.broadcastMessage(message)
}

// HandleTrigger returns an HTTP handler for triggering messages.
func HandleTrigger(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if r.Method != http.MethodPost {
			http.Error(w, "Invalid Request Method", http.StatusMethodNotAllowed)
			return
		}

		var message Message
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			log.Print("error decoding message", err)
			http.Error(w, "Invalid Request Body", http.StatusBadRequest)
			return
		}

		select {
		case <-ctx.Done():
			http.Error(w, "Timeout", http.StatusRequestTimeout)
			return
		default:
			hub.Trigger(message)
		}

		w.WriteHeader(http.StatusOK)
	}
}

// RemoveClient removes a client from all channels and the hub.
func (h *Hub) RemoveClient(client *Client) {
	client.channels.Range(func(key, _ interface{}) bool {
		channel := key.(string)
		h.unregister <- &Subscription{Client: client, Channel: channel}
		return true
	})

	h.clients.Delete(client)
	close(client.send)
}
