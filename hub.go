package pushpop

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
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
	clients    map[*Client]bool
	broadcast  chan Message
	register   chan *Subscription
	unregister chan *Subscription
	channels   map[string]map[*Client]bool
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan Message),
		register:   make(chan *Subscription),
		unregister: make(chan *Subscription),
		channels:   make(map[string]map[*Client]bool),
		clients:    make(map[*Client]bool),
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
	clients := h.channels[sub.Channel]
	if clients == nil {
		clients = make(map[*Client]bool)
		h.channels[sub.Channel] = clients
	}
	clients[sub.Client] = true
	sub.Client.channels[sub.Channel] = true
}

func (h *Hub) removeSubscription(sub *Subscription) {
	clients := h.channels[sub.Channel]
	if clients != nil {
		if _, ok := clients[sub.Client]; ok {
			delete(clients, sub.Client)
			delete(sub.Client.channels, sub.Channel)
			if len(clients) == 0 {
				delete(h.channels, sub.Channel)
			}
		}
	}
}

func (h *Hub) broadcastMessage(message Message) {
	clients := h.channels[message.Channel]
	if clients == nil {
		log.Print("No clients in channel", "channel", message.Channel)
		return
	}
	for client := range clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(clients, client)
		}
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
	for channel := range client.channels {
		h.unregister <- &Subscription{Client: client, Channel: channel}
	}
	delete(h.clients, client)
	close(client.send)
}
