# Advanced Setup

## Uninstall

Remove the symlinks, the install prefix, and the systemd service:

```bash
# remove symlinks
sudo rm -f /usr/local/bin/omni
sudo rm -f /usr/local/bin/omni-server

# remove install prefix
sudo rm -rf /opt/omni/

# stop and remove the systemd service
sudo systemctl disable --now omni-server
sudo rm -f /etc/systemd/system/omni-server.service
sudo systemctl daemon-reload
```

After running these commands `omni` and `omni-server` are fully removed from the system.
