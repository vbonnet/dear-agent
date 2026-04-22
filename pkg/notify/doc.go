// Package notify provides a notification dispatch layer on top of the eventbus.
//
// It implements eventbus.Sink to intercept notification-channel events and
// fan them out to one or more Dispatcher backends (log, webhook, tmux,
// desktop). Dispatchers are configured via YAML and selected at runtime
// based on the notification's severity and channel.
package notify
