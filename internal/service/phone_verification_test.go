package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"pack_mate/internal/config"
)

type pnvsTestTransport func(*http.Request) (*http.Response, error)

// RoundTrip handles a PNVS request without contacting Alibaba Cloud.
func (f pnvsTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestPNVSService(t *testing.T) *AliyunPhoneVerificationService {
	t.Helper()
	s, err := NewAliyunPhoneVerificationService(config.AliyunPNVSConfig{
		AccessKeyID: "test-id", AccessKeySecret: "test-secret", SecurityToken: "test-token",
		SchemeName: "测试方案", SignName: "测试签名", TemplateCode: "100001",
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPNVSSendAndVerifyRequests(t *testing.T) {
	t.Parallel()
	s := newTestPNVSService(t)
	if s.client.Timeout != 10*time.Second {
		t.Fatalf("unexpected default request timeout: %v", s.client.Timeout)
	}
	var nonces []string
	s.client.Transport = pnvsTestTransport(func(req *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		for key, values := range req.Header {
			for _, value := range values {
				headers.Add(key, value)
			}
		}
		req.Header = headers
		if req.Method != http.MethodPost || req.URL.Host != aliyunPNVSEndpoint || req.URL.Scheme != "https" {
			t.Fatalf("unexpected request URL or method: %s %s", req.Method, req.URL)
		}
		req.Form = req.URL.Query()
		for key, want := range map[string]string{
			"SchemeName": "测试方案", "CountryCode": "86", "PhoneNumber": "13800138000",
		} {
			if req.URL.Query().Get(key) != want {
				t.Errorf("unexpected %s: %q", key, req.URL.Query().Get(key))
			}
		}
		if req.Header.Get("x-acs-version") != "2017-05-25" || req.Header.Get("x-acs-security-token") != "test-token" {
			t.Fatalf("missing SDK version or STS token headers: %#v", req.Header)
		}
		if !strings.HasPrefix(req.Header.Get("Authorization"), "ACS3-HMAC-SHA256 Credential=test-id,") {
			t.Fatalf("unexpected SDK authorization: %s", req.Header.Get("Authorization"))
		}
		nonces = append(nonces, req.Header.Get("x-acs-signature-nonce"))
		switch req.Header.Get("x-acs-action") {
		case "SendSmsVerifyCode":
			for key, want := range map[string]string{
				"SignName": "测试签名", "TemplateCode": "100001", "TemplateParam": `{"code":"##code##","min":"5"}`,
				"CodeType": "1", "CodeLength": "6", "ValidTime": "300", "Interval": "60",
				"DuplicatePolicy": "1", "ReturnVerifyCode": "false",
			} {
				if req.Form.Get(key) != want {
					t.Errorf("unexpected %s: %q", key, req.Form.Get(key))
				}
			}
		case "CheckSmsVerifyCode":
			if req.Form.Get("VerifyCode") != "123456" || req.Form.Has("TemplateParam") {
				t.Fatal("unexpected verification parameters")
			}
		default:
			t.Fatal("unexpected action")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Code":"OK","Success":true,"Model":{"VerifyResult":"PASS"}}`))}, nil
	})
	if err := s.SendCode(context.Background(), " +8613800138000 "); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.VerifyCode(context.Background(), "13800138000", "123456"); !ok || err != nil {
		t.Fatalf("VerifyCode = %v, %v", ok, err)
	}
	if len(nonces) != 2 || nonces[0] == "" || nonces[0] == nonces[1] {
		t.Fatalf("unexpected nonces: %v", nonces)
	}
}

func TestPNVSUsesConfiguredDeliveryPolicy(t *testing.T) {
	t.Parallel()
	s, err := NewAliyunPhoneVerificationService(config.AliyunPNVSConfig{
		AccessKeyID: "test-id", AccessKeySecret: "test-secret",
		SignName: "test-sign", TemplateCode: "100001",
		RequestTimeoutSeconds: 15, ValidTimeSeconds: 240, IntervalSeconds: 90, DuplicatePolicy: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.client.Timeout != 15*time.Second {
		t.Fatalf("unexpected request timeout: %v", s.client.Timeout)
	}
	s.client.Transport = pnvsTestTransport(func(req *http.Request) (*http.Response, error) {
		for key, want := range map[string]string{
			"ValidTime": "240", "Interval": "90", "DuplicatePolicy": "2",
			"TemplateParam": `{"code":"##code##","min":"4"}`,
		} {
			if got := req.URL.Query().Get(key); got != want {
				t.Errorf("unexpected %s: %q", key, got)
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"Code":"OK","Success":true}`))}, nil
	})
	if err := s.SendCode(context.Background(), "13800138000"); err != nil {
		t.Fatal(err)
	}
}

func TestPNVSResponseFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{"wrong code", 200, `{"Code":"OK","Success":true,"Model":{"VerifyResult":"UNKNOWN"}}`, false},
		{"missing model", 200, `{"Code":"OK","Success":true}`, true},
		{"empty result", 200, `{"Code":"OK","Success":true,"Model":{}}`, true},
		{"unknown result", 200, `{"Code":"OK","Success":true,"Model":{"VerifyResult":"NEW"}}`, true},
		{"business failure", 200, `{"Code":"FREQUENCY_FAIL","Success":false,"Message":"secret-code"}`, true},
		{"false success", 200, `{"Code":"OK","Success":false,"Model":{"VerifyResult":"PASS"}}`, true},
		{"http failure", 403, `{"Code":"OK","Success":true,"Model":{"VerifyResult":"PASS"}}`, true},
		{"invalid json", 200, `not json`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestPNVSService(t)
			s.client.Transport = pnvsTestTransport(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			ok, err := s.VerifyCode(context.Background(), "13800138000", "123456")
			if ok || (err != nil) != tc.wantError {
				t.Fatalf("VerifyCode = %v, %v", ok, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-code") {
				t.Fatal("response message leaked")
			}
			if tc.wantError && tc.name != "missing model" && tc.name != "empty result" && tc.name != "unknown result" {
				if err := s.SendCode(context.Background(), "13800138000"); err == nil {
					t.Fatal("expected send error")
				}
			}
		})
	}
}

func TestPNVSRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	s := newTestPNVSService(t)
	s.client.Transport = pnvsTestTransport(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("invalid input reached transport")
		return nil, errors.New("unexpected request")
	})
	for _, phone := range []string{"", "+12025550123", "1380013800", "abcdefghijk", "１３８００１３８０００"} {
		if err := s.SendCode(context.Background(), phone); err == nil {
			t.Errorf("accepted phone %q", phone)
		}
		if _, err := s.VerifyCode(context.Background(), phone, "123456"); err == nil {
			t.Errorf("accepted verification phone %q", phone)
		}
	}
	for _, code := range []string{"", "12345", "1234567", "abcdef", "１２３４５６"} {
		if _, err := s.VerifyCode(context.Background(), "13800138000", code); err == nil {
			t.Errorf("accepted code %q", code)
		}
	}
	for _, cfg := range []config.AliyunPNVSConfig{
		{},
		{AccessKeyID: "id", AccessKeySecret: "secret"},
		{AccessKeyID: "id", AccessKeySecret: "secret", SignName: "sign", TemplateCode: "template", SchemeName: strings.Repeat("字", 21)},
		{AccessKeyID: "id", AccessKeySecret: "secret", SignName: "sign", TemplateCode: "template", DuplicatePolicy: 3},
	} {
		if _, err := NewAliyunPhoneVerificationService(cfg); err == nil {
			t.Fatal("accepted invalid config")
		}
	}
}

func TestPNVSTransportErrors(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("connection failed")} {
		s := newTestPNVSService(t)
		s.client.Transport = pnvsTestTransport(func(_ *http.Request) (*http.Response, error) {
			return nil, cause
		})
		if err := s.SendCode(context.Background(), "13800138000"); err == nil {
			t.Fatalf("expected wrapped cause, got %v", err)
		}
		if ok, err := s.VerifyCode(context.Background(), "13800138000", "123456"); ok || err == nil {
			t.Fatalf("VerifyCode = %v, %v", ok, err)
		}
	}
}

func TestPNVSContextCancellation(t *testing.T) {
	t.Parallel()
	s := newTestPNVSService(t)
	started := make(chan struct{})
	s.client.Transport = pnvsTestTransport(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- s.SendCode(ctx, "13800138000") }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request was not canceled")
	}
	if _, err := s.VerifyCode(ctx, "13800138000", "123456"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context to prevent verification, got %v", err)
	}
}

func TestPNVSRequestErrorPreservesCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("request URL contains secret-code")
	err := pnvsCallError(context.Background(), "send code", cause)
	if !errors.Is(err, cause) || strings.Contains(err.Error(), "secret-code") {
		t.Fatalf("unexpected error wrapping: %v", err)
	}
}

func TestPNVSDefaultSchemeRequests(t *testing.T) {
	t.Parallel()
	s := newTestPNVSService(t)
	s.config.SchemeName = ""
	s.client.Transport = pnvsTestTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Has("SchemeName") {
			t.Fatal("default scheme must omit SchemeName rather than send an empty value")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Code":"OK","Success":true,"Model":{"VerifyResult":"PASS"}}`))}, nil
	})
	if err := s.SendCode(context.Background(), "13800138000"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.VerifyCode(context.Background(), "13800138000", "123456"); !ok || err != nil {
		t.Fatalf("VerifyCode = %v, %v", ok, err)
	}
}

func TestPNVSSendFailureDiagnostics(t *testing.T) {
	t.Parallel()
	s := newTestPNVSService(t)
	s.client.Transport = pnvsTestTransport(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Code":"lc.ASSERT_ERROR","Success":false,"Message":"provider diagnostic secret-code","RequestId":"test-request-id"}`))}, nil
	})
	err := s.SendCode(context.Background(), "13800138000")
	var responseErr *pnvsResponseError
	if !errors.As(err, &responseErr) {
		t.Fatalf("expected provider response error, got %v", err)
	}
	if responseErr.code != "lc.ASSERT_ERROR" || responseErr.message != "provider diagnostic secret-code" || responseErr.requestID != "test-request-id" {
		t.Fatal("provider diagnostics were not preserved")
	}
	if !strings.Contains(err.Error(), "test-request-id") || strings.Contains(err.Error(), "secret-code") {
		t.Fatalf("unexpected public error: %v", err)
	}
}

func TestPNVSRateLimitClassification(t *testing.T) {
	t.Parallel()
	for _, code := range []string{"FREQUENCY_FAIL", "BUSINESS_LIMIT_CONTROL"} {
		err := &pnvsResponseError{action: "send code", code: code, requestID: "test-request-id"}
		if !errors.Is(err, errPhoneCodeRateLimited) {
			t.Fatalf("expected %s to be classified as rate limited", code)
		}
	}
	err := &pnvsResponseError{action: "send code", code: "OTHER_ERROR", requestID: "test-request-id"}
	if errors.Is(err, errPhoneCodeRateLimited) {
		t.Fatal("unexpected rate-limit classification")
	}
}
