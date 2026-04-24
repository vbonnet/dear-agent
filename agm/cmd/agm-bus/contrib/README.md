# agm-bus deployment contrib

Templates and install tooling for running `agm-bus` as a long-lived
local daemon. These sit outside `agm/cmd/agm-bus/` main package because
they're deployment artifacts, not Go code.

## macOS (launchd)

```sh
# 1. Install the broker binary first.
GOWORK=off go install github.com/vbonnet/dear-agent/agm/cmd/agm-bus@latest

# 2. Install and load the launchd agent.
./install-launchd.sh install

# 3. Verify.
./install-launchd.sh status
agm-bus status    # probes the unix socket
```

The agent lives at `~/Library/LaunchAgents/com.vbonnet.agm-bus.plist`,
runs as the current user, starts at login (`RunAtLoad`), restarts on
crash (`KeepAlive` with 10-second throttle), and logs to
`~/.agm/logs/agm-bus.log`.

To pass custom flags (verbose logging, non-default socket path, etc.),
edit the installed plist under `~/Library/LaunchAgents/` directly and
reload:

```sh
launchctl bootout gui/$(id -u)/com.vbonnet.agm-bus
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.vbonnet.agm-bus.plist
```

Uninstall:

```sh
./install-launchd.sh uninstall
```

## Linux (systemd user unit)

```sh
# 1. Install the broker binary first.
GOWORK=off go install github.com/vbonnet/dear-agent/agm/cmd/agm-bus@latest

# 2. Install and start the systemd user service.
./install-systemd.sh install

# 3. Verify.
./install-systemd.sh status
agm-bus status    # probes the unix socket
```

The unit lives at `~/.config/systemd/user/agm-bus.service`, runs as the
current user, starts at login (`WantedBy=default.target`), restarts on
crash (`Restart=on-failure` with 10-second throttle), and logs to
`~/.agm/logs/agm-bus.log`.

To pass custom flags (verbose logging, non-default socket path, etc.),
use a drop-in override rather than editing the installed unit directly:

```sh
systemctl --user edit agm-bus
# Add under [Service]:
#   Environment=AGM_VERBOSE=1
```

Reload and restart:

```sh
systemctl --user daemon-reload
systemctl --user restart agm-bus
```

Uninstall:

```sh
./install-systemd.sh uninstall
```

`install-systemd.sh` is a no-op when `systemctl` is not present (e.g.
macOS), so it is safe to call from cross-platform install helpers.

## Not a daemon?

For interactive dev, skip the launchd agent and just run the broker in
a terminal:

```sh
agm-bus serve -verbose
```
