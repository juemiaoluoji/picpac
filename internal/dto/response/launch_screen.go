package response

// LaunchScreenResponse defines the public launch screen configuration.
type LaunchScreenResponse struct {
	Enabled     bool   `json:"enabled"`
	Revision    string `json:"revision"`
	Type        string `json:"type"`
	MediaURL    string `json:"media_url"`
	PosterURL   string `json:"poster_url"`
	DurationMS  int    `json:"duration_ms"`
	SkipEnabled bool   `json:"skip_enabled"`
}
