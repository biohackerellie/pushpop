package pushpop

import (
	"log"
	"net/http"
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
		select {
		default:
			var message map[string]interface{}
			if err := c.conn.ReadJSON(&message); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("WebSocket closed by client: %v", c.conn.RemoteAddr())
				} else {
					log.Printf("Error reading JSON from client: %v, err: %v", c.conn.RemoteAddr(), err)
				}
				return // Exit on read error
			}

			action, _ := message["action"].(string)
			channel, _ := message["channel"].(string)

			if action == "" || channel == "" {
				continue
			}

			switch action {
			case "ping":
				if err := c.conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"pong"}`)); err != nil {
					log.Printf("Error sending pong to client: %v, err: %v", c.conn.RemoteAddr(), err)
					return
				}

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
			}
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
