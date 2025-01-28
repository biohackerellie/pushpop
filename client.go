package pushpop

import (
	"encoding/json"
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
	log      Logger
}

// Constants for WebSocket timeouts.
const (
	writeWait      = 10 * time.Second
	pongWait       = 30 * time.Second
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
			hub.log.Error("Failed to upgrade connection", "err", err)
			return
		}
		conn.SetReadLimit(maxMessageSize)
		if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
			hub.log.Error("Error setting read deadline", "err", err)
		}
		conn.SetPongHandler(func(string) error {
			if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
				hub.log.Error("Error setting read deadline", "err", err)
			}
			return nil
		})

		client := &Client{
			hub:      hub,
			conn:     conn,
			send:     make(chan Message, 256),
			channels: sync.Map{},
			log:      hub.log,
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
				c.log.Info("WebSocket closed by client", "addr", c.conn.RemoteAddr())
			} else if strings.Contains(err.Error(), "connection reset by peer") {
				c.log.Warn("Connection reset by peer for client. Cleaning up.", "client", c.conn.RemoteAddr())
			} else {
				c.log.Warn("Error reading message from client", "client", c.conn.RemoteAddr(), "err", err)
			}
			return
		}

		if messageType != websocket.TextMessage {
			c.log.Warn("Unsupported message type from client", "client", c.conn.RemoteAddr(), "message", messageType)
			continue
		}

		// Parse the message as JSON
		var message map[string]interface{}
		if err := json.Unmarshal(rawMessage, &message); err != nil {
			c.log.Warn("Invalid JSON from client", "client", c.conn.RemoteAddr(), "message", string(rawMessage), "err", err)
			continue
		}

		// Extract the action and handle it
		action, _ := message["action"].(string)
		channel, _ := message["channel"].(string)

		switch action {
		case "ping":
			// Respond to client heartbeat
			if err := c.conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"pong"}`)); err != nil {
				c.log.Error("Error sending pong to client", "client", c.conn.RemoteAddr(), "err", err)
				return
			}
		case "subscribe":
			if channel == "" {
				c.log.Warn("Client attempted to subscribe without specifying a channel.", "client", c.conn.RemoteAddr())
				continue
			}
			c.hub.register <- &Subscription{Client: c, Channel: channel}
			c.log.Debug("Client subscribed to channel", "client", c.conn.RemoteAddr(), "channel", channel)
		case "unsubscribe":
			if channel == "" {
				c.log.Warn("Client attempted to unsubscribe without specifying a channel.", "client", c.conn.RemoteAddr())
				continue
			}
			c.hub.unregister <- &Subscription{Client: c, Channel: channel}
			c.log.Debug("Client unsubscribed from channel", "client", c.conn.RemoteAddr(), "channel", channel)
		case "message":
			payload := message["payload"]
			if channel == "" {
				c.log.Warn("Client attempted to send a message without specifying a channel.", "client", c.conn.RemoteAddr())
				continue
			}
			msg := Message{
				Channel: channel,
				Event:   "message",
				Payload: payload,
			}
			c.hub.broadcast <- msg
			c.log.Debug("Client sent a message to channel", "client", c.conn.RemoteAddr(), "channel", channel)
		default:
			c.log.Error("Unhandled action from client", "action", action, "client", c.conn.RemoteAddr())
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
				c.log.Error("Error setting write deadline", "err", err)
			}
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.log.Debug("WebSocket closed by client")
				} else {
					c.log.Error("Error writing JSON", "err", err)
				}
				return
			}
		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.log.Error("Error setting write deadline", "err", err)
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					c.log.Debug("WebSocket closed by client")
				} else {
					c.log.Error("Error sending ping", "err", err)
				}
				return
			}
		}
	}
}
