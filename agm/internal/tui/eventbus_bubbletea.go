// Package tui provides tui functionality.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// EventReceivedMsg is a Bubble Tea message sent when an event is received
type EventReceivedMsg struct {
	Event *eventbus.Event
}

// ConnectionStatusMsg is a Bubble Tea message sent when connection status changes
type ConnectionStatusMsg struct {
	Connected bool
	Error     error
}

// WaitForEventCmd returns a Bubble Tea command that waits for the next event
func WaitForEventCmd(client *EventBusClient) tea.Cmd {
	return func() tea.Msg {
		event := <-client.Listen()
		return EventReceivedMsg{Event: event}
	}
}

// CheckConnectionCmd returns a Bubble Tea command that checks connection status
func CheckConnectionCmd(client *EventBusClient) tea.Cmd {
	return func() tea.Msg {
		return ConnectionStatusMsg{
			Connected: client.IsConnected(),
		}
	}
}
