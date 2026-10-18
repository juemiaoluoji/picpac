package config

// ProfileUploadConfig limits avatar upload size and decoded image dimensions.
type ProfileUploadConfig struct {
	MaxBytes  int64 `json:"max_bytes"`
	MaxPixels int64 `json:"max_pixels"`
}

// WithDefaults fills unspecified profile upload limits.
func (c ProfileUploadConfig) WithDefaults() ProfileUploadConfig {
	if c.MaxBytes <= 0 {
		c.MaxBytes = 5 << 20
	}
	if c.MaxPixels <= 0 {
		c.MaxPixels = 20_000_000
	}
	return c
}
