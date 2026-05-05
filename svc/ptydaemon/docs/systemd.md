# omni-server systemd service

## Service name
`omni-server`

## Key environment variables
| Variable            | Default                    | Purpose                          |
|---------------------|----------------------------|----------------------------------|
| PTYDAEMON_SOCKET    | /tmp/ptydaemon.sock        | Unix socket path for IPC         |
| PTYDAEMON_DB        | /tmp/ptydaemon.db          | SQLite database path             |
| PTYDAEMON_PID       | /tmp/omni-server.pid       | PID file path                    |
| DEV                 | (unset)                    | Set to any value for debug logs  |

## Install as a systemd user service
```bash
# copy and fill in the template
cp svc/ptydaemon/ptydaemon.service.template ~/.config/systemd/user/omni-server.service
systemctl --user daemon-reload
systemctl --user enable omni-server
systemctl --user start omni-server
```

## Common commands
```bash
omni server start          # start daemon (direct or via systemd)
omni server start -d       # start with debug logging (DEV=1)
omni server stop           # stop daemon
omni server status         # print active sessions and daemon uptime

systemctl --user status omni-server
journalctl --user -u omni-server -f     # follow logs
journalctl --user -u omni-server -n 50  # last 50 lines
```

## Check socket is live
```bash
curl --unix-socket /tmp/ptydaemon.sock http://localhost/status
```
