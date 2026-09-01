package metadataworker

// TagMappingExport is the worker-facing projection of tag mappings.
type TagMappingExport struct {
	Aliases   []string `json:"aliases,omitempty"`
	Type      string   `json:"type,omitempty"`
	MaxLength int      `json:"maxLength,omitempty"`
	Split     []string `json:"split,omitempty"`
	Album     bool     `json:"album,omitempty"`
}

// WorkerScanConfig is sent to the Lofty metadata worker for Rust-side tag clean and PID.
type WorkerScanConfig struct {
	TagMappings           map[string]TagMappingExport `json:"tag_mappings,omitempty"`
	ArtistSplitExceptions []string                    `json:"artist_split_exceptions,omitempty"`
	ArtistsSplit          []string                    `json:"artists_split,omitempty"`
	RolesSplit            []string                    `json:"roles_split,omitempty"`
	ArtistJoiner          string                      `json:"artist_joiner,omitempty"`
	PIDConfig             map[string]any              `json:"pid_config,omitempty"`
	LibraryID             int                         `json:"library_id,omitempty"`
}
