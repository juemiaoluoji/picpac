package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"pack_mate/internal/domain"
	"pack_mate/internal/dto/request"
	"pack_mate/internal/service"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type fakeItemService struct{}

func (s *fakeItemService) CreateItem(_ context.Context, _ request.CreateItemInput) (*domain.Item, error) {
	return &domain.Item{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "item"}, nil
}

func (s *fakeItemService) CreateItemsBatch(_ context.Context, _ request.BatchCreateItemsInput) ([]domain.Item, error) {
	return []domain.Item{{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "item"}}, nil
}

func (s *fakeItemService) ListItems(_ context.Context, _ request.ListItemsInput) ([]domain.Item, error) {
	return []domain.Item{}, nil
}

func (s *fakeItemService) GetItem(_ context.Context, _ string, _ string) (*domain.Item, error) {
	return &domain.Item{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "item"}, nil
}

func (s *fakeItemService) UpdateItem(_ context.Context, _ string, _ string, _ request.UpdateItemInput) (*domain.Item, error) {
	return &domain.Item{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "item"}, nil
}

func (s *fakeItemService) DeleteItem(_ context.Context, _ string, _ string) error {
	return nil
}

type fakeCategoryService struct{}

func (s *fakeCategoryService) ListCategories(_ context.Context) ([]domain.Category, error) {
	return []domain.Category{{ID: bson.NewObjectID(), Key: "other", Name: "其他"}}, nil
}

func (s *fakeCategoryService) GetCategory(_ context.Context, _ string) (*domain.Category, error) {
	return &domain.Category{ID: bson.NewObjectID(), Key: "other", Name: "其他"}, nil
}

func (s *fakeCategoryService) GetCategoryByID(_ context.Context, _ bson.ObjectID) (*domain.Category, error) {
	return &domain.Category{ID: bson.NewObjectID(), Key: "other", Name: "其他"}, nil
}

func (s *fakeCategoryService) ResolveCategoryID(_ context.Context, _ string) (bson.ObjectID, error) {
	return bson.NewObjectID(), nil
}

type fakeAISuggestionService struct{}

func (s *fakeAISuggestionService) RecommendPackItems(_ context.Context, _ request.RecommendPackItemsInput) ([]service.RecommendedItem, error) {
	return []service.RecommendedItem{}, nil
}

func (s *fakeAISuggestionService) GenerateItemDrafts(_ context.Context, _ request.GenerateItemDraftsInput) ([]service.ItemDraft, error) {
	return []service.ItemDraft{}, nil
}

type fakePackService struct{}

func (s *fakePackService) CreatePack(_ context.Context, _ request.CreatePackInput) (*domain.Pack, error) {
	return &domain.Pack{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "pack"}, nil
}

func (s *fakePackService) ListPacks(_ context.Context, _ request.ListPacksInput) ([]domain.Pack, error) {
	return []domain.Pack{}, nil
}

func (s *fakePackService) GetPack(_ context.Context, _ string, _ string) (*domain.Pack, error) {
	return &domain.Pack{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "pack"}, nil
}

func (s *fakePackService) UpdatePackProfile(_ context.Context, _ string, _ string, _ request.UpdatePackProfileInput) (*domain.Pack, error) {
	return &domain.Pack{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "pack"}, nil
}

func (s *fakePackService) AddPackItems(_ context.Context, _ string, _ string, _ request.AddPackItemsInput) (*domain.Pack, error) {
	return &domain.Pack{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "pack"}, nil
}

func (s *fakePackService) RemovePackItems(_ context.Context, _ string, _ string, _ request.RemovePackItemsInput) (*domain.Pack, error) {
	return &domain.Pack{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "pack"}, nil
}

func (s *fakePackService) DeletePack(_ context.Context, _ string, _ string) error {
	return nil
}

type fakeChecklistService struct{}

func (s *fakeChecklistService) CreateChecklist(_ context.Context, _ request.CreateChecklistInput) (*domain.Checklist, error) {
	return &domain.Checklist{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "checklist"}, nil
}

func (s *fakeChecklistService) ListChecklists(_ context.Context, _ request.ListChecklistsInput) ([]domain.Checklist, error) {
	return []domain.Checklist{}, nil
}

func (s *fakeChecklistService) GetChecklist(_ context.Context, _ string, _ string) (*domain.Checklist, error) {
	return &domain.Checklist{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "checklist"}, nil
}

func (s *fakeChecklistService) UpdateChecklist(_ context.Context, _ string, _ string, _ request.UpdateChecklistInput) (*domain.Checklist, error) {
	return &domain.Checklist{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "checklist"}, nil
}

func (s *fakeChecklistService) AddChecklistLineItems(_ context.Context, _ string, _ string, _ request.AddChecklistLineItemsInput) (*domain.Checklist, error) {
	return &domain.Checklist{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "checklist"}, nil
}

func (s *fakeChecklistService) RemoveChecklistLineItems(_ context.Context, _ string, _ string, _ request.RemoveChecklistLineItemsInput) (*domain.Checklist, error) {
	return &domain.Checklist{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "checklist"}, nil
}

func (s *fakeChecklistService) UpdateChecklistLineItemStatus(_ context.Context, _ string, _ string, _ string, _ request.UpdateChecklistLineItemStatusInput) (*domain.Checklist, error) {
	return &domain.Checklist{ID: bson.NewObjectID(), UserID: bson.NewObjectID(), Name: "checklist"}, nil
}

func (s *fakeChecklistService) DeleteChecklist(_ context.Context, _ string, _ string) error {
	return nil
}

type fakeAuthService struct{}

type fakeLaunchScreenService struct{}

// Get returns a disabled launch screen for route tests.
func (s *fakeLaunchScreenService) Get(_ context.Context) (*domain.LaunchScreen, error) {
	return &domain.LaunchScreen{}, nil
}

func (s *fakeAuthService) SendPhoneCode(_ context.Context, _ request.SendPhoneCodeInput) error {
	return nil
}

func (s *fakeAuthService) LoginWithPhone(_ context.Context, _ request.PhoneLoginInput) (*service.AuthResult, error) {
	return &service.AuthResult{AccessToken: "access", RefreshToken: "refresh", User: &domain.User{ID: bson.NewObjectID(), Profile: domain.UserProfile{Username: "user", AvatarObjectKey: "user-avatar/default.jpg"}, Status: domain.UserStatusCreated}}, nil
}

func (s *fakeAuthService) LoginWithPhonePassword(_ context.Context, _ request.PhonePasswordLoginInput) (*service.AuthResult, error) {
	return &service.AuthResult{AccessToken: "access", RefreshToken: "refresh", User: &domain.User{ID: bson.NewObjectID(), Profile: domain.UserProfile{Username: "user", AvatarObjectKey: "user-avatar/default.jpg"}, Status: domain.UserStatusCreated}}, nil
}

func (s *fakeAuthService) Refresh(_ context.Context, _ request.RefreshTokenInput) (*service.RefreshResult, error) {
	return &service.RefreshResult{AccessToken: "access"}, nil
}

func (s *fakeAuthService) Logout(_ context.Context, _ request.LogoutInput) error {
	return nil
}

func (s *fakeAuthService) SetupPassword(_ context.Context, _ request.SetupPasswordInput) error {
	return nil
}

func (s *fakeAuthService) ChangePassword(_ context.Context, _ request.ChangePasswordInput) error {
	return nil
}

func (s *fakeAuthService) GetSecurity(_ context.Context, _ string) (*domain.AuthSecurity, error) {
	return &domain.AuthSecurity{Phone: "138****8888", PasswordSetup: true}, nil
}

func (s *fakeAuthService) Me(_ context.Context, _ string) (*domain.User, error) {
	return &domain.User{ID: bson.NewObjectID(), Profile: domain.UserProfile{Username: "user", AvatarObjectKey: "user-avatar/default.jpg"}, Status: domain.UserStatusCreated}, nil
}

func (s *fakeAuthService) UpdateMyProfile(_ context.Context, _ string, _ request.UpdateMyProfileInput) (*domain.User, error) {
	return &domain.User{ID: bson.NewObjectID(), Profile: domain.UserProfile{Username: "user", AvatarObjectKey: "user-avatar/default.jpg"}, Status: domain.UserStatusCreated}, nil
}

func (s *fakeAuthService) DeleteMe(_ context.Context, _ string) error {
	return nil
}

type fakeTokenService struct{}

func (s *fakeTokenService) CreateAccessToken(_ bson.ObjectID) (string, error) {
	return "access", nil
}

func (s *fakeTokenService) ParseAccessToken(_ string) (bson.ObjectID, error) {
	return bson.NewObjectID(), nil
}

func (s *fakeTokenService) CreateRefreshToken() (string, string, error) {
	return "refresh", "hash", nil
}

func (s *fakeTokenService) HashToken(_ string) string {
	return "hash"
}

type fakeObjectURLSigner struct{}

func (s *fakeObjectURLSigner) SignGetURL(_ context.Context, objectKey string) (string, error) {
	return "https://signed.example/" + objectKey, nil
}

func TestRegisterAPIRoutesExposesEndpoints(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerAPIRoutes(router, &fakeItemService{}, &fakePackService{}, &fakeChecklistService{}, &fakeCategoryService{}, &fakeLaunchScreenService{}, &fakeAISuggestionService{}, &fakeAuthService{}, &fakeTokenService{}, &fakeObjectURLSigner{})

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/ai/item-drafts"},
		{method: http.MethodPost, path: "/api/v1/ai/pack/item-recommendations"},
		{method: http.MethodPost, path: "/api/v1/item"},
		{method: http.MethodPost, path: "/api/v1/item/batch"},
		{method: http.MethodGet, path: "/api/v1/item"},
		{method: http.MethodGet, path: "/api/v1/item/" + bson.NewObjectID().Hex()},
		{method: http.MethodPut, path: "/api/v1/item/" + bson.NewObjectID().Hex()},
		{method: http.MethodDelete, path: "/api/v1/item/" + bson.NewObjectID().Hex()},
		{method: http.MethodGet, path: "/api/v1/categories"},
		{method: http.MethodGet, path: "/api/v1/app/launch-screen"},
		{method: http.MethodPost, path: "/api/v1/pack"},
		{method: http.MethodGet, path: "/api/v1/pack"},
		{method: http.MethodGet, path: "/api/v1/pack/" + bson.NewObjectID().Hex()},
		{method: http.MethodPatch, path: "/api/v1/pack/" + bson.NewObjectID().Hex() + "/profile"},
		{method: http.MethodPost, path: "/api/v1/pack/" + bson.NewObjectID().Hex() + "/items"},
		{method: http.MethodDelete, path: "/api/v1/pack/" + bson.NewObjectID().Hex() + "/items"},
		{method: http.MethodDelete, path: "/api/v1/pack/" + bson.NewObjectID().Hex()},
		{method: http.MethodPost, path: "/api/v1/checklist"},
		{method: http.MethodGet, path: "/api/v1/checklist"},
		{method: http.MethodGet, path: "/api/v1/checklist/" + bson.NewObjectID().Hex()},
		{method: http.MethodPut, path: "/api/v1/checklist/" + bson.NewObjectID().Hex()},
		{method: http.MethodPost, path: "/api/v1/checklist/" + bson.NewObjectID().Hex() + "/items"},
		{method: http.MethodDelete, path: "/api/v1/checklist/" + bson.NewObjectID().Hex() + "/items"},
		{method: http.MethodPatch, path: "/api/v1/checklist/" + bson.NewObjectID().Hex() + "/items/" + bson.NewObjectID().Hex() + "/status"},
		{method: http.MethodDelete, path: "/api/v1/checklist/" + bson.NewObjectID().Hex()},
		{method: http.MethodPost, path: "/api/v1/auth/phone/code"},
		{method: http.MethodPost, path: "/api/v1/auth/phone/login"},
		{method: http.MethodPost, path: "/api/v1/auth/phone/code/login"},
		{method: http.MethodPost, path: "/api/v1/auth/phone/password/login"},
		{method: http.MethodPost, path: "/api/v1/auth/refresh"},
		{method: http.MethodPost, path: "/api/v1/auth/logout"},
		{method: http.MethodPost, path: "/api/v1/auth/password/setup"},
		{method: http.MethodPost, path: "/api/v1/auth/password/reset"},
		{method: http.MethodPut, path: "/api/v1/auth/password"},
		{method: http.MethodGet, path: "/api/v1/auth/security"},
		{method: http.MethodDelete, path: "/api/v1/auth/me"},
		{method: http.MethodGet, path: "/api/v1/me"},
		{method: http.MethodPut, path: "/api/v1/me/profile"},
		{method: http.MethodPatch, path: "/api/v1/me/profile"},
	}

	for _, test := range tests {
		req := httptest.NewRequest(test.method, test.path, bytes.NewBuffer(nil))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code == http.StatusNotFound {
			t.Fatalf("expected route %s %s to be registered", test.method, test.path)
		}
	}
}

func TestRegisterAPIRoutesDoesNotExposeLegacyPackUpdate(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerAPIRoutes(router, &fakeItemService{}, &fakePackService{}, &fakeChecklistService{}, &fakeCategoryService{}, &fakeLaunchScreenService{}, &fakeAISuggestionService{}, &fakeAuthService{}, &fakeTokenService{}, &fakeObjectURLSigner{})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/pack/"+bson.NewObjectID().Hex(), bytes.NewBuffer(nil))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected legacy pack update route to be unregistered, got %d", recorder.Code)
	}
}

// ResetPassword implements password reset for HTTP tests.
func (s *fakeAuthService) ResetPassword(_ context.Context, _ service.ResetPasswordInput) error {
	return nil
}

// PatchMyProfile implements profile patching for HTTP tests.
func (s *fakeAuthService) PatchMyProfile(_ context.Context, _ string, _ service.ProfilePatchInput) (*domain.User, error) {
	return &domain.User{}, nil
}
