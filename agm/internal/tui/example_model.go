package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// ExampleModel demonstrates how to integrate EventBusClient with Bubble Tea
// This is an example implementation showing the pattern for TUI integration
type ExampleModel struct {
	eventClient   *EventBusClient
	lastEvent     *eventbus.Event
	notifications []string
	connected     bool
	width         int
	height        int
}

// NewExampleModel creates a new example TUI model
func NewExampleModel(eventBusURL string) *ExampleModel {
	return &ExampleModel{
		eventClient:   NewEventBusClient(eventBusURL),
		notifications: make([]string, 0),
		connected:     false,
	}
}

// Init initializes the model
func (m *ExampleModel) Init() tea.Cmd {
	// Connect to event bus
	if err := m.eventClient.Connect(m.eventClient.url); err != nil {
		return nil
	}

	// Subscribe to all sessions (use "*" for all)
	if err := m.eventClient.Subscribe("*"); err != nil {
		return nil
	}

	// Start listening for events
	return tea.Batch(
		WaitForEventCmd(m.eventClient),
		CheckConnectionCmd(m.eventClient),
	)
}

// Update handles messages
func (m *ExampleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case EventReceivedMsg:
		// Handle received event
		m.lastEvent = msg.Event
		m.addNotification(formatEventNotification(msg.Event))

		// Continue listening for events
		return m, WaitForEventCmd(m.eventClient)

	case ConnectionStatusMsg:
		m.connected = msg.Connected

		// Check connection status periodically
		return m, tea.Tick(5e9, func(time.Time) tea.Msg {
			return CheckConnectionCmd(m.eventClient)()
		})

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "c":
			// Clear notifications
			m.notifications = make([]string, 0)
			return m, nil
		}
	}

	return m, nil
}

// View renders the model
func (m *ExampleModel) View() string {
	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		Padding(1, 2)
	b.WriteString(titleStyle.Render("AGM Event Monitor"))
	b.WriteString("\n\n")

	// Connection status
	statusStyle := lipgloss.NewStyle().Padding(0, 2)
	if m.connected {
		connectedStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)
		b.WriteString(statusStyle.Render(connectedStyle.Render("● Connected")))
	} else {
		disconnectedStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)
		b.WriteString(statusStyle.Render(disconnectedStyle.Render("○ Disconnected")))
	}
	b.WriteString("\n\n")

	// Notifications
	notificationStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1, 2).
		Width(80)

	b.WriteString(notificationStyle.Render(renderNotifications(m.notifications)))
	b.WriteString("\n\n")

	// Help
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(1, 2)
	b.WriteString(helpStyle.Render("Press 'c' to clear notifications | 'q' to quit"))

	return b.String()
}

// addNotification adds a notification to the list
func (m *ExampleModel) addNotification(notification string) {
	m.notifications = append(m.notifications, notification)

	// Keep only last 10 notifications
	if len(m.notifications) > 10 {
		m.notifications = m.notifications[len(m.notifications)-10:]
	}
}

// formatEventNotification formats an event as a notification string
func formatEventNotification(event *eventbus.Event) string {
	var icon string
	var color lipgloss.Color

	switch event.Type {
	case eventbus.EventSessionEscalated:
		icon = "🚨"
		color = lipgloss.Color("9") // Red
	case eventbus.EventSessionStuck:
		icon = "⏸️"
		color = lipgloss.Color("11") // Yellow
	case eventbus.EventSessionRecovered:
		icon = "✅"
		color = lipgloss.Color("10") // Green
	case eventbus.EventSessionStateChange:
		icon = "🔄"
		color = lipgloss.Color("12") // Blue
	case eventbus.EventSessionCompleted:
		icon = "🎉"
		color = lipgloss.Color("10") // Green
	default:
		icon = "ℹ️"
		color = lipgloss.Color("8") // Gray
	}

	style := lipgloss.NewStyle().Foreground(color).Bold(true)
	typeStr := style.Render(string(event.Type))

	return fmt.Sprintf("%s %s [%s] @ %s",
		icon,
		typeStr,
		event.SessionID,
		event.Timestamp.Format("15:04:05"))
}

// renderNotifications renders the notification list
func renderNotifications(notifications []string) string {
	if len(notifications) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Italic(true)
		return emptyStyle.Render("No notifications yet...")
	}

	var b strings.Builder
	b.WriteString("Recent Events:\n\n")

	for i := len(notifications) - 1; i >= 0; i-- {
		b.WriteString(notifications[i])
		b.WriteString("\n")
	}

	return b.String()
}
