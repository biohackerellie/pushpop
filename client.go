package pushpop

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a WebSocket client.
type Client struct {
	channels sync.Map
	hub      *Hub
	conn     *websocket.Conn
	send     chan Message
}

// Constants for WebSocket timeouts.
const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

// upgrader upgrades HTTP connections to WebSocket connections.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

// ServeWs handles WebSocket requests from clients.
func ServeWs(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("Failed to upgrade connection", "err", err)
			return
		}
		conn.SetReadLimit(maxMessageSize)
		if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
			log.Print("Error setting read deadline", "err", err)
		}
		conn.SetPongHandler(func(string) error {
			if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
				log.Print("Error setting read deadline", "err", err)
			}
			return nil
		})

		client := &Client{
			hub:      hub,
			conn:     conn,
			send:     make(chan Message, 256),
			channels: sync.Map{},
		}

		hub.clients.Store(client, true)

		go client.writePump()
		go client.readPump()
	}
}

// readPump reads messages from the WebSocket connection.
func (c *Client) readPump() {
	defer func() {
		c.hub.RemoveClient(c) // Unregister the client from the hub
		c.conn.Close()        // Close the WebSocket connection
	}()

	for {
		messageType, rawMessage, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("WebSocket closed by client: %v", c.conn.RemoteAddr())
			} else if strings.Contains(err.Error(), "connection reset by peer") {
				log.Printf("Connection reset by peer for client: %v. Cleaning up.", c.conn.RemoteAddr())
			} else {
				log.Printf("Error reading message from client: %v, err: %v", c.conn.RemoteAddr(), err)
			}
			return
		}

		if messageType != websocket.TextMessage {
			log.Printf("Unsupported message type from client: %v, type: %d", c.conn.RemoteAddr(), messageType)
			continue
		}

		// Parse the message as JSON
		var message map[string]interface{}
		if err := json.Unmarshal(rawMessage, &message); err != nil {
			log.Printf("Invalid JSON from client: %v, message: %s, err: %v", c.conn.RemoteAddr(), string(rawMessage), err)
			continue
		}

		// Extract the action and handle it
		action, _ := message["action"].(string)
		channel, _ := message["channel"].(string)

		switch action {
		case "ping":
			// Respond to client heartbeat
			if err := c.conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"pong"}`)); err != nil {
				log.Printf("Error sending pong to client: %v, err: %v", c.conn.RemoteAddr(), err)
				return
			}
		case "subscribe":
			if channel == "" {
				log.Printf("Client %v attempted to subscribe without specifying a channel.", c.conn.RemoteAddr())
				continue
			}
			c.hub.register <- &Subscription{Client: c, Channel: channel}
			log.Printf("Client %v subscribed to channel: %s", c.conn.RemoteAddr(), channel)
		case "unsubscribe":
			if channel == "" {
				log.Printf("Client %v attempted to unsubscribe without specifying a channel.", c.conn.RemoteAddr())
				continue
			}
			c.hub.unregister <- &Subscription{Client: c, Channel: channel}
			log.Printf("Client %v unsubscribed from channel: %s", c.conn.RemoteAddr(), channel)
		case "message":
			payload := message["payload"]
			if channel == "" {
				log.Printf("Client %v attempted to send a message without specifying a channel.", c.conn.RemoteAddr())
				continue
			}
			msg := Message{
				Channel: channel,
				Event:   "message",
				Payload: payload,
			}
			c.hub.broadcast <- msg
			log.Printf("Client %v sent a message to channel: %s", c.conn.RemoteAddr(), channel)
		default:
			log.Printf("Unhandled action: %s from client: %v", action, c.conn.RemoteAddr())
		}
	}
}

// writePump writes messages to the WebSocket connection.

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Print("Error setting write deadline", "err", err)
			}
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Print("WebSocket closed by client")
				} else {
					log.Print("Error writing JSON", "err", err)
				}
				return
			}
		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Print("Error setting write deadline", "err", err)
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Print("WebSocket closed by client")
				} else {
					log.Print("Error sending ping", "err", err)
				}
				return
			}
		}
	}
}
