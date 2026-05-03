// Package eventbus provides eventbus functionality.
package eventbus

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

const (
	// Default configuration values
	defaultPort       = 8080
	defaultMaxClients = 100

	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now
		// In production, you might want to restrict this
		return true
	},
}

// Broadcaster defines the interface for broadcasting events
type Broadcaster interface {
	Broadcast(event *Event)
}

// Hub maintains the set of active clients and broadcasts messages to the clients
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Inbound messages from the clients
	broadcast chan *Event

	// Register requests from the clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Mutex for thread-safe operations
	mu sync.RWMutex

	// Max clients allowed
	maxClients int

	// Shutdown channel
	shutdown chan struct{}

	// Logger for structured logging
	logger *slog.Logger
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	maxClients := defaultMaxClients
	if maxStr := os.Getenv("AGM_EVENTBUS_MAX_CLIENTS"); maxStr != "" {
		if max, err := strconv.Atoi(maxStr); err == nil && max > 0 {
			maxClients = max
		}
	}

	return &Hub{
		broadcast:  make(chan *Event, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		maxClients: maxClients,
		shutdown:   make(chan struct{}),
		logger:     logging.DefaultLogger(),
	}
}

// Run starts the hub's event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if len(h.clients) >= h.maxClients {
				h.mu.Unlock()
				// Send error and close connection immediately
				errMsg := map[string]string{"error": "maximum clients reached"}
				if data, err := json.Marshal(errMsg); err == nil {
					client.conn.SetWriteDeadline(time.Now().Add(writeWait))
					client.conn.WriteMessage(websocket.TextMessage, data)
				}
				client.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				client.conn.Close()
				h.logger.Warn("Rejected client connection", "max_clients", h.maxClients)
				continue
			}
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Info("Client registered", "total", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.logger.Info("Client unregistered", "total", len(h.clients))
			}
			h.mu.Unlock()

		case event := <-h.broadcast:
			// We may delete entries from h.clients on full-buffer disconnects,
			// so take the write lock for the whole iteration.
			h.mu.Lock()
			for client := range h.clients {
				// Filter by session if client has a filter set
				if client.sessionFilter != "*" && client.sessionFilter != event.SessionID {
					continue
				}

				// Marshal event to JSON
				data, err := json.Marshal(event)
				if err != nil {
					h.logger.Warn("Failed to marshal event", "error", err)
					continue
				}

				select {
				case client.send <- data:
				default:
					// Client's send channel is full, close it
					close(client.send)
					delete(h.clients, client)
					h.logger.Warn("Client send buffer full, disconnected")
				}
			}
			h.mu.Unlock()

		case <-h.shutdown:
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				client.conn.Close()
			}
			h.clients = make(map[*Client]bool)
			h.mu.Unlock()
			h.logger.Info("Hub shutdown complete")
			return
		}
	}
}

// Broadcast sends an event to all connected clients
func (h *Hub) Broadcast(event *Event) {
	select {
	case h.broadcast <- event:
	default:
		h.logger.Warn("Broadcast channel full, event dropped")
	}
}

// Shutdown gracefully shuts down the hub
func (h *Hub) Shutdown() {
	close(h.shutdown)
}

// ClientCount returns the current number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Client represents a WebSocket connection
type Client struct {
	hub *Hub

	// The websocket connection
	conn *websocket.Conn

	// Buffered channel of outbound messages
	send chan []byte

	// Session filter: "*" for all sessions, or specific session ID
	sessionFilter string
}

// ClientMessage represents a message from the client
type ClientMessage struct {
	Action    string `json:"action"`     // "subscribe" or "unsubscribe"
	SessionID string `json:"session_id"` // "*" for all or specific session ID
}

// readPump pumps messages from the websocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.logger.Warn("WebSocket error", "error", err)
			}
			break
		}

		// Parse client message
		var msg ClientMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			// Send error response
			errMsg := map[string]string{"error": "invalid message format"}
			if data, err := json.Marshal(errMsg); err == nil {
				c.send <- data
			}
			continue
		}

		// Handle actions
		switch msg.Action {
		case "subscribe":
			if msg.SessionID == "" {
				msg.SessionID = "*"
			}
			c.hub.mu.Lock()
			c.sessionFilter = msg.SessionID
			filter := c.sessionFilter
			c.hub.mu.Unlock()
			c.hub.logger.Info("Client subscribed to session", "session_filter", filter)

		case "unsubscribe":
			c.hub.mu.Lock()
			c.sessionFilter = ""
			c.hub.mu.Unlock()
			c.hub.logger.Info("Client unsubscribed")

		default:
			errMsg := map[string]string{"error": "unknown action: " + msg.Action}
			if data, err := json.Marshal(errMsg); err == nil {
				c.send <- data
			}
		}
	}
}

// writePump pumps messages from the hub to the websocket connection
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
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send each message as a separate WebSocket frame
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWebSocket handles websocket requests from the peer
func ServeWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		hub.logger.Warn("WebSocket upgrade failed", "error", err)
		return
	}

	client := &Client{
		hub:           hub,
		conn:          conn,
		send:          make(chan []byte, 256),
		sessionFilter: "*", // Default to all sessions
	}

	client.hub.register <- client

	// Start client goroutines
	go client.writePump()
	go client.readPump()
}

// GetPort returns the port to use for the WebSocket server
func GetPort() int {
	if portStr := os.Getenv("AGM_EVENTBUS_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil && port > 0 && port < 65536 {
			return port
		}
	}
	return defaultPort
}
