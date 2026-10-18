package main

import (
	"testing"

	"pack_mate/internal/config"
	"pack_mate/internal/service"
)

func TestPhoneVerificationProviderSelection(t *testing.T) {
	t.Parallel()
	provider, err := newPhoneVerificationService(&config.Configuration{Env: "dev"})
	if err != nil || provider != nil {
		t.Fatal("dev must not create a provider or require Alibaba Cloud credentials")
	}
	for _, env := range []string{"prod", "", "production"} {
		if _, err := newPhoneVerificationService(&config.Configuration{Env: env}); err == nil {
			t.Fatalf("expected invalid config error for environment %q", env)
		}
	}
	provider, err = newPhoneVerificationService(&config.Configuration{
		Env: "prod",
		Auth: config.AuthConfig{PhoneCode: config.AliyunPNVSConfig{
			AccessKeyID: "test-id", AccessKeySecret: "test-secret", SignName: "test-sign", TemplateCode: "100001",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(*service.AliyunPhoneVerificationService); !ok {
		t.Fatal("prod must use the real PNVS implementation")
	}
}
