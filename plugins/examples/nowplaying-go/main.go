package main

import (
	"encoding/json"
	"fmt"

	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/lifecycle"
	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
	"github.com/navidrome/navidrome/plugins/pdk/go/scheduler"
)

const scheduleID = "nowplaying-check"

type nowPlayingPlugin struct{}

func init() {
	plugin := &nowPlayingPlugin{}
	lifecycle.Register(plugin)
	scheduler.Register(plugin)
}

var (
	_ lifecycle.InitProvider     = (*nowPlayingPlugin)(nil)
	_ scheduler.CallbackProvider = (*nowPlayingPlugin)(nil)
)

func (*nowPlayingPlugin) OnInit() error {
	cron := configOrDefault("cron", "*/1 * * * *")
	pdk.Log(pdk.LogInfo, "Now Playing Logger initializing with cron: "+cron)

	newScheduleID, err := host.SchedulerScheduleRecurring(cron, "check", scheduleID)
	if err != nil {
		return fmt.Errorf("failed to schedule task: %w", err)
	}
	pdk.Log(pdk.LogInfo, "Scheduled recurring task with ID: "+newScheduleID)
	return nil
}

func (*nowPlayingPlugin) OnCallback(input scheduler.SchedulerCallbackRequest) error {
	if input.ScheduleID != scheduleID {
		return nil
	}

	user := configOrDefault("user", "admin")
	responseJSON, err := host.SubsonicAPICall("getNowPlaying?u=" + user)
	if err != nil {
		pdk.Log(pdk.LogError, "Failed to get now playing: "+err.Error())
		return nil
	}

	var response nowPlayingResponse
	if err := json.Unmarshal([]byte(responseJSON), &response); err != nil {
		pdk.Log(pdk.LogError, "Failed to parse now playing response: "+err.Error())
		return nil
	}

	entries := response.SubsonicResponse.NowPlaying.Entry
	if len(entries) == 0 {
		pdk.Log(pdk.LogInfo, "No users currently playing music")
		return nil
	}

	for _, entry := range entries {
		pdk.Log(pdk.LogInfo, fmt.Sprintf("%s is playing: %s - %s (%s)",
			valueOrDefault(entry.Username, "Unknown User"),
			valueOrDefault(entry.Artist, "Unknown Artist"),
			valueOrDefault(entry.Title, "Unknown Title"),
			valueOrDefault(entry.Album, "Unknown Album"),
		))
	}
	return nil
}

func configOrDefault(key, fallback string) string {
	if value, ok := pdk.GetConfig(key); ok && value != "" {
		return value
	}
	return fallback
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

type nowPlayingResponse struct {
	SubsonicResponse struct {
		NowPlaying struct {
			Entry nowPlayingEntries `json:"entry"`
		} `json:"nowPlaying"`
	} `json:"subsonic-response"`
}

type nowPlayingEntries []nowPlayingEntry

func (e *nowPlayingEntries) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*e = nil
		return nil
	}
	if data[0] == '[' {
		var entries []nowPlayingEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			return err
		}
		*e = entries
		return nil
	}

	var entry nowPlayingEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return err
	}
	*e = []nowPlayingEntry{entry}
	return nil
}

type nowPlayingEntry struct {
	Artist   string `json:"artist"`
	Title    string `json:"title"`
	Album    string `json:"album"`
	Username string `json:"username"`
}

func main() {}
