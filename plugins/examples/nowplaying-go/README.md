# Now Playing Go Plugin

Schedules a recurring task and logs currently playing tracks through Navidrome's Subsonic API host service.

```bash
make nowplaying-go.ndp
```

Configuration:

```toml
[PluginConfig.nowplaying-go]
cron = "*/1 * * * *"
user = "admin"
```
