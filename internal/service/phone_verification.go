package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"pack_mate/internal/config"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dypnsapi "github.com/alibabacloud-go/dypnsapi-20170525/v3/client"
	"github.com/alibabacloud-go/tea/dara"
)

const aliyunPNVSEndpoint = "dypnsapi.aliyuncs.com"

var errPhoneCodeRateLimited = errors.New("phone code rate limited")

// PhoneVerificationService defines provider-managed SMS verification behavior.
type PhoneVerificationService interface {
	SendCode(ctx context.Context, phone string) error
	VerifyCode(ctx context.Context, phone string, code string) (bool, error)
}

// AliyunPhoneVerificationService sends and verifies codes with Alibaba Cloud PNVS.
type AliyunPhoneVerificationService struct {
	config config.AliyunPNVSConfig
	client *http.Client
}

type pnvsHTTPClient struct {
	ctx    context.Context
	client *http.Client
}

type pnvsRequestError struct {
	action string
	cause  error
}

type pnvsResponseError struct {
	action    string
	code      string
	message   string
	requestID string
}

// Is classifies provider throttling responses without exposing provider-specific codes upstream.
func (e *pnvsResponseError) Is(target error) bool {
	return target == errPhoneCodeRateLimited && isPNVSRateLimitCode(e.code)
}

// Error describes a provider failure without exposing its raw message.
func (e *pnvsResponseError) Error() string {
	return fmt.Sprintf("aliyun pnvs %s failed: code %s, request ID %s", e.action, e.code, e.requestID)
}

// Error describes the failed operation without exposing the SDK request URL.
func (e *pnvsRequestError) Error() string {
	return fmt.Sprintf("aliyun pnvs %s failed (%T)", e.action, e.cause)
}

// Unwrap preserves the underlying SDK error for callers.
func (e *pnvsRequestError) Unwrap() error {
	return e.cause
}

// Call attaches the caller context to the SDK request.
func (c *pnvsHTTPClient) Call(req *http.Request, _ *http.Transport) (*http.Response, error) {
	return c.client.Do(req.WithContext(c.ctx))
}

var _ PhoneVerificationService = (*AliyunPhoneVerificationService)(nil)

// NewAliyunPhoneVerificationService creates a phone verification service.
func NewAliyunPhoneVerificationService(cfg config.AliyunPNVSConfig) (*AliyunPhoneVerificationService, error) {
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.AccessKeySecret = strings.TrimSpace(cfg.AccessKeySecret)
	cfg.SecurityToken = strings.TrimSpace(cfg.SecurityToken)
	cfg.SchemeName = strings.TrimSpace(cfg.SchemeName)
	cfg.SignName = strings.TrimSpace(cfg.SignName)
	cfg.TemplateCode = strings.TrimSpace(cfg.TemplateCode)
	if cfg.RequestTimeoutSeconds <= 0 {
		cfg.RequestTimeoutSeconds = 10
	}
	if cfg.ValidTimeSeconds <= 0 {
		cfg.ValidTimeSeconds = 300
	}
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = 60
	}
	if cfg.DuplicatePolicy == 0 {
		cfg.DuplicatePolicy = 1
	}
	switch {
	case cfg.AccessKeyID == "" || cfg.AccessKeySecret == "":
		return nil, fmt.Errorf("aliyun pnvs access key is required")
	case cfg.SignName == "" || cfg.TemplateCode == "":
		return nil, fmt.Errorf("aliyun pnvs sign name and template code are required")
	case utf8.RuneCountInString(cfg.SchemeName) > 20:
		return nil, fmt.Errorf("aliyun pnvs scheme name must not exceed 20 characters")
	case cfg.DuplicatePolicy != 1 && cfg.DuplicatePolicy != 2:
		return nil, fmt.Errorf("aliyun pnvs duplicate policy must be 1 or 2")
	}

	return &AliyunPhoneVerificationService{
		config: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeoutSeconds) * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// SendCode sends a provider-generated six-digit code using the configured delivery policy.
func (s *AliyunPhoneVerificationService) SendCode(ctx context.Context, phone string) error {
	phone, err := normalizePNVSPhone(phone)
	if err != nil {
		return err
	}
	client, err := s.newClient(ctx)
	if err != nil {
		return err
	}
	// Provider verification requires the placeholder instead of a locally generated code.
	resp, err := client.SendSmsVerifyCodeWithOptions(&dypnsapi.SendSmsVerifyCodeRequest{
		PhoneNumber:      dara.String(phone),
		CountryCode:      dara.String("86"),
		SchemeName:       s.schemeName(),
		SignName:         dara.String(s.config.SignName),
		TemplateCode:     dara.String(s.config.TemplateCode),
		TemplateParam:    dara.String(s.templateParam()),
		CodeType:         dara.Int64(1),
		CodeLength:       dara.Int64(6),
		ValidTime:        dara.Int64(s.config.ValidTimeSeconds),
		Interval:         dara.Int64(s.config.IntervalSeconds),
		DuplicatePolicy:  dara.Int64(s.config.DuplicatePolicy),
		ReturnVerifyCode: dara.Bool(false),
	}, &dara.RuntimeOptions{Autoretry: dara.Bool(false)})
	if err != nil {
		return pnvsCallError(ctx, "send code", err)
	}
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("aliyun pnvs response did not contain send result")
	}
	if dara.Int32Value(resp.StatusCode) != http.StatusOK || dara.StringValue(resp.Body.Code) != "OK" || !dara.BoolValue(resp.Body.Success) {
		return &pnvsResponseError{
			action: "send code", code: dara.StringValue(resp.Body.Code),
			message: dara.StringValue(resp.Body.Message), requestID: dara.StringValue(resp.Body.RequestId),
		}
	}
	return nil
}

// VerifyCode returns whether Alibaba Cloud accepted the supplied SMS code.
func (s *AliyunPhoneVerificationService) VerifyCode(ctx context.Context, phone string, code string) (bool, error) {
	phone, err := normalizePNVSPhone(phone)
	if err != nil {
		return false, err
	}
	code = strings.TrimSpace(code)
	if len(code) != 6 || !pnvsDigits(code) {
		return false, fmt.Errorf("verification code must contain six digits")
	}
	client, err := s.newClient(ctx)
	if err != nil {
		return false, err
	}
	resp, err := client.CheckSmsVerifyCodeWithOptions(&dypnsapi.CheckSmsVerifyCodeRequest{
		PhoneNumber: dara.String(phone),
		CountryCode: dara.String("86"),
		SchemeName:  s.schemeName(),
		VerifyCode:  dara.String(code),
	}, &dara.RuntimeOptions{Autoretry: dara.Bool(false)})
	if err != nil {
		return false, pnvsCallError(ctx, "verify code", err)
	}
	if resp == nil || resp.Body == nil {
		return false, fmt.Errorf("aliyun pnvs response did not contain verification result")
	}
	if dara.Int32Value(resp.StatusCode) != http.StatusOK || dara.StringValue(resp.Body.Code) != "OK" || !dara.BoolValue(resp.Body.Success) {
		return false, fmt.Errorf("aliyun pnvs verify code failed: code %s", dara.StringValue(resp.Body.Code))
	}
	if resp.Body.Model == nil {
		return false, fmt.Errorf("aliyun pnvs response did not contain verification result")
	}
	switch dara.StringValue(resp.Body.Model.VerifyResult) {
	case "PASS":
		return true, nil
	case "UNKNOWN":
		return false, nil
	default:
		return false, fmt.Errorf("aliyun pnvs response contained invalid verification result")
	}
}

func (s *AliyunPhoneVerificationService) schemeName() *string {
	// Omit the optional parameter so PNVS selects its default scheme.
	if s.config.SchemeName == "" {
		return nil
	}
	return dara.String(s.config.SchemeName)
}

func (s *AliyunPhoneVerificationService) templateParam() string {
	minutes := (s.config.ValidTimeSeconds + 59) / 60
	return fmt.Sprintf(`{"code":"##code##","min":"%d"}`, minutes)
}

func (s *AliyunPhoneVerificationService) newClient(ctx context.Context) (*dypnsapi.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Each SDK client owns its request context; the HTTP connection pool remains shared.
	client, err := dypnsapi.NewClient(&openapi.Config{
		AccessKeyId:     dara.String(s.config.AccessKeyID),
		AccessKeySecret: dara.String(s.config.AccessKeySecret),
		SecurityToken:   dara.String(s.config.SecurityToken),
		Endpoint:        dara.String(aliyunPNVSEndpoint),
		Protocol:        dara.String("HTTPS"),
		RetryOptions:    &dara.RetryOptions{Retryable: false},
		HttpClient:      &pnvsHTTPClient{ctx: ctx, client: s.client},
	})
	if err != nil {
		return nil, fmt.Errorf("create aliyun pnvs client: %w", err)
	}
	return client, nil
}

func pnvsCallError(ctx context.Context, action string, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("aliyun pnvs %s: %w", action, ctx.Err())
	}
	// SDK errors can contain the signed URL with the phone and verification code.
	return &pnvsRequestError{action: action, cause: err}
}

func normalizePNVSPhone(phone string) (string, error) {
	phone = strings.TrimPrefix(strings.TrimSpace(phone), "+86")
	if len(phone) != 11 || phone[0] != '1' || !pnvsDigits(phone) {
		return "", fmt.Errorf("phone must be a mainland China mobile number")
	}
	return phone, nil
}

func pnvsDigits(value string) bool {
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func isPNVSRateLimitCode(code string) bool {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "FREQUENCY_FAIL", "BUSINESS_LIMIT_CONTROL":
		return true
	default:
		return false
	}
}
