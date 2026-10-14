package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPhoneVerificationEnvironment(t *testing.T) {
	t.Parallel()
	for _, env := range []string{"dev", "prod", "", "production"} {
		t.Run(env, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			body := fmt.Sprintf(`{
				"env":%q,
				"server":{"port":"8080"},
				"oss":{"endpoint":"test","bucket_name":"test","access_key_id":"test","access_key_secret":"test","public_base_url":"test"},
				"dashscope":{"api_key":"test","image_model":"test"},
				"deepseek":{"api_key":"test","model":"test","base_url":"test"},
				"mongo":{"uri":"test","database":"test"},
				"auth":{"access_token_secret":"test","phone_code":{"dev_fixed_code":"654321","access_key_id":"pnvs-id","access_key_secret":"pnvs-secret","security_token":"pnvs-token","scheme_name":"scheme","sign_name":"sign","template_code":"100001","request_timeout_seconds":15,"valid_time_seconds":240,"interval_seconds":90,"duplicate_policy":2}}
			}`, env)
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if env != "dev" && env != "prod" {
				if err == nil {
					t.Fatal("invalid environment must not enable dev authentication")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Env != env || cfg.Auth.PhoneCode != (AliyunPNVSConfig{
				DevFixedCode: "654321",
				AccessKeyID:  "pnvs-id", AccessKeySecret: "pnvs-secret", SecurityToken: "pnvs-token",
				SchemeName: "scheme", SignName: "sign", TemplateCode: "100001",
				RequestTimeoutSeconds: 15, ValidTimeSeconds: 240, IntervalSeconds: 90, DuplicatePolicy: 2,
			}) {
				t.Fatal("environment or PNVS settings were not loaded")
			}
		})
	}
}
