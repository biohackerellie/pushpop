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
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
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

// readPump reads messages from the WebSocket connection.
func (c *Client) readPump() {
	defer func() {
		c.hub.RemoveClient(c) // Unregister the client from the hub
		c.conn.Close()        // Close the WebSocket connection
	}()

	for {
		messageType, rawMessage, err := c.conn.ReadMessage()
		if err != nil {
			// Handle normal WebSocket close events
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("WebSocket closed by client: %v", c.conn.RemoteAddr())
			} else if strings.Contains(err.Error(), "connection reset by peer") {
				// Suppress log for expected errors
				log.Printf("Connection reset by peer for client: %v. Cleaning up.", c.conn.RemoteAddr())
			} else {
				log.Printf("Error reading message from client: %v, err: %v", c.conn.RemoteAddr(), err)
			}
			return // Exit on any read error
		}

		// Handle valid messages
		switch messageType {
		case websocket.TextMessage:
			// Process text messages
			var message map[string]interface{}
			if err := json.Unmarshal(rawMessage, &message); err != nil {
				log.Printf("Invalid JSON from client: %v, message: %s, err: %v", c.conn.RemoteAddr(), string(rawMessage), err)
				continue // Skip invalid JSON
			}
			action, _ := message["action"].(string)

			switch action {
			case "ping":
				if err := c.conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"pong"}`)); err != nil {
					return
				}
			default:
				log.Printf("Unhandled action: %s", action)
			}

		case websocket.PingMessage:
			// Reply to WebSocket ping frames
			if err := c.conn.WriteMessage(websocket.PongMessage, nil); err != nil {
				log.Printf("Error responding to ping from client: %v, err: %v", c.conn.RemoteAddr(), err)
				return
			}

		default:
			log.Printf("Unsupported message type from client: %v, type: %d", c.conn.RemoteAddr(), messageType)
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
