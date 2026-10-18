package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Configuration defines the application runtime configuration loaded from config.json.
type Configuration struct {
	Env       string          `json:"env"`
	Server    ServerConfig    `json:"server"`
	OSS       OSSConfig       `json:"oss"`
	Dashscope DashscopeConfig `json:"dashscope"`
	Deepseek  DeepseekConfig  `json:"deepseek"`
	CORS      CORSConfig      `json:"cors"`
	Mongo     MongoConfig     `json:"mongo"`
	Auth      AuthConfig      `json:"auth"`
}

// CategorySeedFile defines system category seeds loaded from category.json.
type CategorySeedFile struct {
	Categories []CategorySeedConfig `json:"categories"`
}

// CategorySeedConfig defines a system category seed.
type CategorySeedConfig struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	Port string `json:"port"`
}

// OSSConfig defines Alibaba Cloud OSS access settings.
type OSSConfig struct {
	Endpoint            string `json:"endpoint"`
	BucketName          string `json:"bucket_name"`
	AccessKeyID         string `json:"access_key_id"`
	AccessKeySecret     string `json:"access_key_secret"`
	PublicBaseURL       string `json:"public_base_url"`
	SignedURLTTLSeconds int64  `json:"signed_url_ttl_seconds"`
}

// DashscopeConfig defines DashScope access settings.
type DashscopeConfig struct {
	APIKey     string `json:"api_key"`
	ImageModel string `json:"image_model"`
}

// DeepseekConfig defines DeepSeek chat completion settings.
type DeepseekConfig struct {
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
	BaseURL string `json:"base_url"`
}

// CORSConfig defines allowed origins.
type CORSConfig struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

// MongoConfig defines MongoDB connection settings.
type MongoConfig struct {
	URI                   string `json:"uri"`
	Database              string `json:"database"`
	ConnectTimeoutSeconds int    `json:"connect_timeout_seconds"`
	MaxPoolSize           uint64 `json:"max_pool_size"`
}

// AuthConfig defines authentication settings.
type AuthConfig struct {
	ProfileUpload          ProfileUploadConfig `json:"profile_upload"`
	AccessTokenSecret      string              `json:"access_token_secret"`
	AccessTokenTTLSeconds  int                 `json:"access_token_ttl_seconds"`
	RefreshTokenTTLSeconds int                 `json:"refresh_token_ttl_seconds"`
	PhoneCode              AliyunPNVSConfig    `json:"phone_code"`
	Password               PasswordConfig      `json:"password"`
}

// PasswordConfig defines password credential settings.
type PasswordConfig struct {
	BcryptCost          int `json:"bcrypt_cost"`
	MaxFailedAttempts   int `json:"max_failed_attempts"`
	LockDurationSeconds int `json:"lock_duration_seconds"`
}

// Load reads and validates application configuration from a JSON file.
func Load(path string) (*Configuration, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Configuration
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("decode config file: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadCategorySeeds reads and validates category seed configuration from a JSON file.
func LoadCategorySeeds(path string) (*CategorySeedFile, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read category seed file: %w", err)
	}

	var seeds CategorySeedFile
	if err := json.Unmarshal(body, &seeds); err != nil {
		return nil, fmt.Errorf("decode category seed file: %w", err)
	}
	if err := validateCategorySeeds(&seeds); err != nil {
		return nil, err
	}

	return &seeds, nil
}

func validate(cfg *Configuration) error {
	cfg.Env = strings.TrimSpace(cfg.Env)
	if cfg.Env != "dev" && cfg.Env != "prod" {
		return fmt.Errorf("invalid config: env must be dev or prod")
	}
	switch {
	case strings.TrimSpace(cfg.Server.Port) == "":
		return fmt.Errorf("invalid config: server.port is required")
	case strings.TrimSpace(cfg.OSS.Endpoint) == "":
		return fmt.Errorf("invalid config: oss.endpoint is required")
	case strings.TrimSpace(cfg.OSS.BucketName) == "":
		return fmt.Errorf("invalid config: oss.bucket_name is required")
	case strings.TrimSpace(cfg.OSS.AccessKeyID) == "":
		return fmt.Errorf("invalid config: oss.access_key_id is required")
	case strings.TrimSpace(cfg.OSS.AccessKeySecret) == "":
		return fmt.Errorf("invalid config: oss.access_key_secret is required")
	case strings.TrimSpace(cfg.OSS.PublicBaseURL) == "":
		return fmt.Errorf("invalid config: oss.public_base_url is required")
	case strings.TrimSpace(cfg.Dashscope.APIKey) == "":
		return fmt.Errorf("invalid config: dashscope.api_key is required")
	case strings.TrimSpace(cfg.Dashscope.ImageModel) == "":
		return fmt.Errorf("invalid config: dashscope.image_model is required")
	case strings.TrimSpace(cfg.Deepseek.APIKey) == "":
		return fmt.Errorf("invalid config: deepseek.api_key is required")
	case strings.TrimSpace(cfg.Deepseek.Model) == "":
		return fmt.Errorf("invalid config: deepseek.model is required")
	case strings.TrimSpace(cfg.Deepseek.BaseURL) == "":
		return fmt.Errorf("invalid config: deepseek.base_url is required")
	case strings.TrimSpace(cfg.Mongo.URI) == "":
		return fmt.Errorf("invalid config: mongo.uri is required")
	case strings.TrimSpace(cfg.Mongo.Database) == "":
		return fmt.Errorf("invalid config: mongo.database is required")
	}

	if cfg.Mongo.ConnectTimeoutSeconds <= 0 {
		cfg.Mongo.ConnectTimeoutSeconds = 10
	}
	if cfg.OSS.SignedURLTTLSeconds <= 0 {
		cfg.OSS.SignedURLTTLSeconds = 3600
	}
	if strings.TrimSpace(cfg.Auth.AccessTokenSecret) == "" {
		return fmt.Errorf("invalid config: auth.access_token_secret is required")
	}
	if cfg.Auth.AccessTokenTTLSeconds <= 0 {
		cfg.Auth.AccessTokenTTLSeconds = 7200
	}
	if cfg.Auth.RefreshTokenTTLSeconds <= 0 {
		cfg.Auth.RefreshTokenTTLSeconds = 2592000
	}
	cfg.Auth.PhoneCode.DevFixedCode = strings.TrimSpace(cfg.Auth.PhoneCode.DevFixedCode)
	if cfg.Auth.PhoneCode.DevFixedCode == "" {
		cfg.Auth.PhoneCode.DevFixedCode = "123456"
	}
	if len(cfg.Auth.PhoneCode.DevFixedCode) != 6 || !asciiDigits(cfg.Auth.PhoneCode.DevFixedCode) {
		return fmt.Errorf("invalid config: auth.phone_code.dev_fixed_code must contain six digits")
	}
	if cfg.Auth.PhoneCode.RequestTimeoutSeconds <= 0 {
		cfg.Auth.PhoneCode.RequestTimeoutSeconds = 10
	}
	if cfg.Auth.PhoneCode.ValidTimeSeconds <= 0 {
		cfg.Auth.PhoneCode.ValidTimeSeconds = 300
	}
	if cfg.Auth.PhoneCode.IntervalSeconds <= 0 {
		cfg.Auth.PhoneCode.IntervalSeconds = 60
	}
	if cfg.Auth.PhoneCode.DuplicatePolicy == 0 {
		cfg.Auth.PhoneCode.DuplicatePolicy = 1
	}
	if cfg.Auth.PhoneCode.DuplicatePolicy != 1 && cfg.Auth.PhoneCode.DuplicatePolicy != 2 {
		return fmt.Errorf("invalid config: auth.phone_code.duplicate_policy must be 1 or 2")
	}
	if cfg.Auth.Password.BcryptCost <= 0 {
		cfg.Auth.Password.BcryptCost = 12
	}
	if cfg.Auth.Password.MaxFailedAttempts <= 0 {
		cfg.Auth.Password.MaxFailedAttempts = 5
	}
	if cfg.Auth.Password.LockDurationSeconds <= 0 {
		cfg.Auth.Password.LockDurationSeconds = 900
	}

	return nil
}

func asciiDigits(value string) bool {
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func validateCategorySeeds(seeds *CategorySeedFile) error {
	if len(seeds.Categories) == 0 {
		return fmt.Errorf("invalid category seed: categories are required")
	}

	seen := make(map[string]struct{}, len(seeds.Categories))
	hasOther := false
	for _, category := range seeds.Categories {
		key := strings.TrimSpace(category.Key)
		name := strings.TrimSpace(category.Name)
		if key == "" {
			return fmt.Errorf("invalid category seed: category key is required")
		}
		if name == "" {
			return fmt.Errorf("invalid category seed: category name is required")
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("invalid category seed: duplicate category key %s", key)
		}
		seen[key] = struct{}{}
		if key == "other" {
			hasOther = true
		}
	}
	if !hasOther {
		return fmt.Errorf("invalid category seed: other category is required")
	}

	return nil
}
