package pushpop

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a WebSocket client.
type Client struct {
	channels map[string]bool
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

	client := &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan Message, 256),
		channels: make(map[string]bool),
	}

	hub.clients[client] = true

	go client.writePump()
	go client.readPump()
}

// readPump reads messages from the WebSocket connection.
func (c *Client) readPump() {
	defer func() {
		c.hub.RemoveClient(c)
		c.conn.Close()
	}()

	for {
		var message map[string]interface{}
		if err := c.conn.ReadJSON(&message); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Print("WebSocket closed by client")
			} else {
				log.Print("Error reading JSON", "err", err)
			}
			break
		}

		action, _ := message["action"].(string)
		channel, _ := message["channel"].(string)
		log.Print("Received message", "action", action, "channel", channel, "message", message)
		if action == "" || channel == "" {
			continue
		}
		switch action {
		case "subscribe":
			c.hub.register <- &Subscription{Client: c, Channel: channel}
		case "unsubscribe":
			c.hub.unregister <- &Subscription{Client: c, Channel: channel}
		case "message":
			payload := message["payload"]
			msg := Message{
				Channel: channel,
				Event:   "message",
				Payload: payload,
			}
			c.hub.broadcast <- msg
		default:
			// Ignore unknown actions
			log.Print("Unknown action", "action", action)
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
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
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
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
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
