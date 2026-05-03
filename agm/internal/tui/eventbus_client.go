package tui

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
	"github.com/vbonnet/dear-agent/agm/internal/logging"
)

const (
	// Reconnect backoff configuration
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 30 * time.Second
	backoffMultiplier     = 2

	// HTTP polling configuration
	httpPollInterval = 5 * time.Second

	// WebSocket read/write timeouts
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

// EventBusClient manages WebSocket connection to the event bus server
type EventBusClient struct {
	url           string
	sessionID     string
	conn          *websocket.Conn
	events        chan *eventbus.Event
	done          chan struct{}
	reconnect     bool
	reconnectMu   sync.Mutex
	mu            sync.Mutex
	isConnected   bool
	httpFallback  bool
	httpClient    *http.Client
	lastEventTime time.Time
	logger        *slog.Logger
}

// NewEventBusClient creates a new EventBusClient instance
func NewEventBusClient(url string) *EventBusClient {
	return &EventBusClient{
		url:          url,
		events:       make(chan *eventbus.Event, 256),
		done:         make(chan struct{}),
		reconnect:    true,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		httpFallback: false,
		logger:       logging.DefaultLogger(),
	}
}

// Connect establishes a WebSocket connection to the server
func (c *EventBusClient) Connect(url string) error {
	c.mu.Lock()
	c.url = url
	c.mu.Unlock()

	// Try WebSocket first
	if err := c.connectWebSocket(); err != nil {
		c.logger.Warn("WebSocket connection failed, falling back to HTTP polling", "error", err)
		c.httpFallback = true
		go c.httpPollLoop()
		return nil // Don't return error, we have HTTP fallback
	}

	c.isConnected = true
	c.httpFallback = false

	// Start read pump
	go c.readPump()
	go c.writePump()

	return nil
}

// connectWebSocket establishes the WebSocket connection
func (c *EventBusClient) connectWebSocket() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(c.url, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("failed to dial WebSocket: %w", err)
	}

	c.conn = conn
	return nil
}

// Subscribe subscribes to events for a specific session
func (c *EventBusClient) Subscribe(sessionID string) error {
	c.mu.Lock()
	c.sessionID = sessionID
	c.mu.Unlock()

	// If using HTTP fallback, no need to send subscribe message
	if c.httpFallback {
		return nil
	}

	// Send subscribe message over WebSocket
	msg := map[string]string{
		"action":     "subscribe",
		"session_id": sessionID,
	}

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("failed to send subscribe message: %w", err)
	}

	return nil
}

// Listen returns a channel that receives events
func (c *EventBusClient) Listen() chan *eventbus.Event {
	return c.events
}

// Close gracefully shuts down the client
func (c *EventBusClient) Close() error {
	c.reconnectMu.Lock()
	c.reconnect = false
	c.reconnectMu.Unlock()

	close(c.done)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		// Send close message
		c.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		c.conn.Close()
		c.conn = nil
	}

	c.isConnected = false
	return nil
}

// IsConnected returns whether the client is currently connected
func (c *EventBusClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isConnected
}

// readPump reads messages from the WebSocket connection
func (c *EventBusClient) readPump() {
	defer func() {
		c.mu.Lock()
		c.isConnected = false
		c.mu.Unlock()

		c.attemptReconnect()
	}()

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return
	}

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		var event eventbus.Event
		if err := conn.ReadJSON(&event); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Warn("WebSocket error", "error", err)
			}
			return
		}

		// Validate event
		if err := event.Validate(); err != nil {
			c.logger.Warn("Received invalid event", "error", err)
			continue
		}

		// Send event to channel (non-blocking)
		select {
		case c.events <- &event:
		case <-c.done:
			return
		default:
			c.logger.Warn("Event channel full, dropping event")
		}
	}
}

// writePump sends ping messages to keep the connection alive
func (c *EventBusClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()

			if conn == nil {
				return
			}

			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.done:
			return
		}
	}
}

// attemptReconnect attempts to reconnect with exponential backoff
func (c *EventBusClient) attemptReconnect() {
	c.reconnectMu.Lock()
	shouldReconnect := c.reconnect
	c.reconnectMu.Unlock()

	if !shouldReconnect {
		return
	}

	delay := initialReconnectDelay

	for {
		// Check if we should stop reconnecting
		c.reconnectMu.Lock()
		shouldReconnect := c.reconnect
		c.reconnectMu.Unlock()

		if !shouldReconnect {
			return
		}

		// Wait before reconnecting
		select {
		case <-time.After(delay):
		case <-c.done:
			return
		}

		c.logger.Info("Attempting to reconnect", "url", c.url)

		// Try to reconnect
		if err := c.connectWebSocket(); err != nil {
			c.logger.Warn("Reconnect failed", "error", err)

			// Increase delay with exponential backoff
			delay *= backoffMultiplier
			if delay > maxReconnectDelay {
				delay = maxReconnectDelay
			}
			continue
		}

		// Successfully reconnected
		c.logger.Info("Successfully reconnected to event bus")

		c.mu.Lock()
		c.isConnected = true
		c.mu.Unlock()

		// Resubscribe to session
		if c.sessionID != "" {
			if err := c.Subscribe(c.sessionID); err != nil {
				c.logger.Warn("Failed to resubscribe", "error", err)
			}
		}

		// Restart read and write pumps
		go c.readPump()
		go c.writePump()

		return
	}
}

// httpPollLoop polls the HTTP endpoint for events when WebSocket is unavailable
func (c *EventBusClient) httpPollLoop() {
	ticker := time.NewTicker(httpPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.pollHTTPEvents()
		case <-c.done:
			return
		}
	}
}

// pollHTTPEvents polls the HTTP endpoint for new events
func (c *EventBusClient) pollHTTPEvents() {
	c.mu.Lock()
	sessionID := c.sessionID
	lastEventTime := c.lastEventTime
	c.mu.Unlock()

	if sessionID == "" {
		return
	}

	// Build HTTP URL from WebSocket URL
	httpURL := c.url
	// Replace ws:// or wss:// with http:// or https://
	if len(httpURL) > 5 && httpURL[:5] == "ws://" {
		httpURL = "http://" + httpURL[5:]
	} else if len(httpURL) > 6 && httpURL[:6] == "wss://" {
		httpURL = "https://" + httpURL[6:]
	}

	// Remove /ws path and add /api/events
	// This is a stub - the HTTP endpoint needs to be implemented separately
	endpoint := fmt.Sprintf("%s/api/events?session_id=%s&since=%d",
		httpURL, sessionID, lastEventTime.Unix())

	resp, err := c.httpClient.Get(endpoint) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		c.logger.Warn("HTTP polling failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("HTTP polling returned non-OK status", "status", resp.StatusCode)
		return
	}

	var events []eventbus.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		c.logger.Warn("Failed to decode events", "error", err)
		return
	}

	// Send events to channel
	for i := range events {
		event := &events[i]

		// Validate event
		if err := event.Validate(); err != nil {
			c.logger.Warn("Received invalid event", "error", err)
			continue
		}

		// Update last event time
		if event.Timestamp.After(lastEventTime) {
			c.mu.Lock()
			c.lastEventTime = event.Timestamp
			c.mu.Unlock()
		}

		// Send event to channel (non-blocking)
		select {
		case c.events <- event:
		case <-c.done:
			return
		default:
			c.logger.Warn("Event channel full, dropping event")
		}
	}
}
