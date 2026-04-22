package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

func TestWaitForEventCmd(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Create a test event
	testEvent, err := eventbus.NewEvent(
		eventbus.EventSessionEscalated,
		"test-session",
		eventbus.SessionEscalatedPayload{
			EscalationType: "error",
			Pattern:        "(?i)error:",
			Line:           "Error: Test",
			LineNumber:     1,
			DetectedAt:     time.Now(),
			Description:    "Test error",
			Severity:       "high",
		},
	)
	require.NoError(t, err)

	// Broadcast event
	err = server.BroadcastEvent(testEvent)
	require.NoError(t, err)

	// Create and execute the command
	cmd := WaitForEventCmd(client)
	msg := cmd()

	// Verify the message
	eventMsg, ok := msg.(EventReceivedMsg)
	require.True(t, ok, "Expected EventReceivedMsg")
	assert.Equal(t, eventbus.EventSessionEscalated, eventMsg.Event.Type)
	assert.Equal(t, "test-session", eventMsg.Event.SessionID)
}

func TestCheckConnectionCmd(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	client := NewEventBusClient(server.URL())
	defer client.Close()

	err := client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Create and execute the command
	cmd := CheckConnectionCmd(client)
	msg := cmd()

	// Verify the message
	statusMsg, ok := msg.(ConnectionStatusMsg)
	require.True(t, ok, "Expected ConnectionStatusMsg")
	assert.True(t, statusMsg.Connected)
	assert.Nil(t, statusMsg.Error)
}

func TestCheckConnectionCmdDisconnected(t *testing.T) {
	client := NewEventBusClient("ws://localhost:9999/ws")
	defer client.Close()

	// Don't connect

	// Create and execute the command
	cmd := CheckConnectionCmd(client)
	msg := cmd()

	// Verify the message
	statusMsg, ok := msg.(ConnectionStatusMsg)
	require.True(t, ok, "Expected ConnectionStatusMsg")
	assert.False(t, statusMsg.Connected)
}

func TestBubbleTeaIntegration(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	// Create a simple test model
	type testModel struct {
		client    *EventBusClient
		lastEvent *eventbus.Event
		connected bool
	}

	model := testModel{
		client: NewEventBusClient(server.URL()),
	}
	defer model.client.Close()

	err := model.client.Connect(server.URL())
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Create a test event
	testEvent, err := eventbus.NewEvent(
		eventbus.EventSessionCompleted,
		"test-session",
		eventbus.SessionCompletedPayload{
			ExitCode:     0,
			Duration:     30 * time.Minute,
			MessageCount: 42,
			FinalState:   "completed",
		},
	)
	require.NoError(t, err)

	// Broadcast event
	err = server.BroadcastEvent(testEvent)
	require.NoError(t, err)

	// Simulate Bubble Tea Update handling
	cmd := WaitForEventCmd(model.client)
	msg := cmd()

	// Handle the message
	switch msg := msg.(type) {
	case EventReceivedMsg:
		model.lastEvent = msg.Event
		assert.Equal(t, eventbus.EventSessionCompleted, msg.Event.Type)
		assert.Equal(t, "test-session", msg.Event.SessionID)
	default:
		t.Fatalf("Unexpected message type: %T", msg)
	}

	// Check connection status
	statusCmd := CheckConnectionCmd(model.client)
	statusMsg := statusCmd()

	switch msg := statusMsg.(type) {
	case ConnectionStatusMsg:
		model.connected = msg.Connected
		assert.True(t, msg.Connected)
	default:
		t.Fatalf("Unexpected message type: %T", msg)
	}
}

func TestExampleModelCreation(t *testing.T) {
	model := NewExampleModel("ws://localhost:8080/ws")
	assert.NotNil(t, model)
	assert.NotNil(t, model.eventClient)
	assert.NotNil(t, model.notifications)
	assert.False(t, model.connected)
}

func TestExampleModelInit(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	model := NewExampleModel(server.URL())

	// Call Init
	cmd := model.Init()
	assert.NotNil(t, cmd)

	// Wait a bit for connection
	time.Sleep(200 * time.Millisecond)

	// Check that client is connected
	assert.True(t, model.eventClient.IsConnected())
}

func TestExampleModelUpdate(t *testing.T) {
	server := NewTestWebSocketServer()
	defer server.Close()

	model := NewExampleModel(server.URL())

	// Test window size message
	sizeMsg := tea.WindowSizeMsg{Width: 100, Height: 50}
	updatedModel, cmd := model.Update(sizeMsg)
	assert.Nil(t, cmd)
	assert.Equal(t, 100, updatedModel.(*ExampleModel).width)
	assert.Equal(t, 50, updatedModel.(*ExampleModel).height)

	// Test event received message
	testEvent, _ := eventbus.NewEvent(
		eventbus.EventSessionEscalated,
		"test-session",
		eventbus.SessionEscalatedPayload{
			EscalationType: "error",
			Pattern:        "error",
			Line:           "Error occurred",
			LineNumber:     1,
			DetectedAt:     time.Now(),
			Description:    "Test error",
			Severity:       "high",
		},
	)

	eventMsg := EventReceivedMsg{Event: testEvent}
	updatedModel, cmd = model.Update(eventMsg)
	assert.NotNil(t, cmd)
	assert.Equal(t, testEvent, updatedModel.(*ExampleModel).lastEvent)
	assert.Equal(t, 1, len(updatedModel.(*ExampleModel).notifications))

	// Test connection status message
	connMsg := ConnectionStatusMsg{Connected: true}
	updatedModel, cmd = model.Update(connMsg)
	assert.NotNil(t, cmd)
	assert.True(t, updatedModel.(*ExampleModel).connected)

	// Test clear notifications key
	clearModel := &ExampleModel{
		notifications: []string{"test1", "test2"},
	}
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	updatedModel, cmd = clearModel.Update(keyMsg)
	assert.Nil(t, cmd)
	assert.Equal(t, 0, len(updatedModel.(*ExampleModel).notifications))

	// Test quit key
	quitMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd = model.Update(quitMsg)
	assert.NotNil(t, cmd)
}

func TestExampleModelView(t *testing.T) {
	model := NewExampleModel("ws://localhost:8080/ws")

	// Test view with no notifications
	view := model.View()
	assert.Contains(t, view, "AGM Event Monitor")
	assert.Contains(t, view, "No notifications yet")

	// Test view with notifications
	model.notifications = []string{"Test notification 1", "Test notification 2"}
	model.connected = true
	view = model.View()
	assert.Contains(t, view, "AGM Event Monitor")
	assert.Contains(t, view, "Connected")
	assert.Contains(t, view, "Test notification 1")
}

func TestAddNotification(t *testing.T) {
	model := NewExampleModel("ws://localhost:8080/ws")

	// Add notifications
	model.addNotification("Notification 1")
	assert.Equal(t, 1, len(model.notifications))

	model.addNotification("Notification 2")
	assert.Equal(t, 2, len(model.notifications))

	// Add more than 10 notifications (should keep only last 10)
	for i := 3; i <= 15; i++ {
		model.addNotification("Notification " + string(rune(i)))
	}

	assert.Equal(t, 10, len(model.notifications))
}

func TestFormatEventNotification(t *testing.T) {
	// Test different event types
	tests := []struct {
		name      string
		eventType eventbus.EventType
		wantIcon  string
	}{
		{
			name:      "escalated",
			eventType: eventbus.EventSessionEscalated,
			wantIcon:  "🚨",
		},
		{
			name:      "stuck",
			eventType: eventbus.EventSessionStuck,
			wantIcon:  "⏸️",
		},
		{
			name:      "recovered",
			eventType: eventbus.EventSessionRecovered,
			wantIcon:  "✅",
		},
		{
			name:      "state_change",
			eventType: eventbus.EventSessionStateChange,
			wantIcon:  "🔄",
		},
		{
			name:      "completed",
			eventType: eventbus.EventSessionCompleted,
			wantIcon:  "🎉",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, _ := eventbus.NewEvent(
				tt.eventType,
				"test-session",
				map[string]string{"test": "data"},
			)

			notification := formatEventNotification(event)
			assert.Contains(t, notification, tt.wantIcon)
			assert.Contains(t, notification, "test-session")
		})
	}
}

func TestRenderNotifications(t *testing.T) {
	// Test with empty notifications
	result := renderNotifications([]string{})
	assert.Contains(t, result, "No notifications yet")

	// Test with notifications
	notifications := []string{
		"Notification 1",
		"Notification 2",
		"Notification 3",
	}
	result = renderNotifications(notifications)
	assert.Contains(t, result, "Recent Events")
	assert.Contains(t, result, "Notification 1")
	assert.Contains(t, result, "Notification 2")
	assert.Contains(t, result, "Notification 3")
}
