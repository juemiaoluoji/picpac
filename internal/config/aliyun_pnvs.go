package config

// AliyunPNVSConfig defines Alibaba Cloud phone number verification settings.
type AliyunPNVSConfig struct {
	DevFixedCode          string `json:"dev_fixed_code"`
	AccessKeyID           string `json:"access_key_id"`
	AccessKeySecret       string `json:"access_key_secret"`
	SecurityToken         string `json:"security_token"`
	SchemeName            string `json:"scheme_name"`
	SignName              string `json:"sign_name"`
	TemplateCode          string `json:"template_code"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	ValidTimeSeconds      int64  `json:"valid_time_seconds"`
	IntervalSeconds       int64  `json:"interval_seconds"`
	// DuplicatePolicy controls repeated sends: 1 replaces the previous code, 2 keeps all unexpired codes valid.
	DuplicatePolicy int64 `json:"duplicate_policy"`
}
