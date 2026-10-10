package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"pack_mate/internal/config"
	"pack_mate/internal/handler"
	mongodb "pack_mate/internal/repository/mongodb"
	"pack_mate/internal/service"
	"pack_mate/internal/service/agent"
	"pack_mate/internal/service/llm"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func newRouter(cfg *config.Configuration, bucket *oss.Bucket, db *mongo.Database) (*gin.Engine, error) {
	verification, err := newPhoneVerificationService(cfg)
	if err != nil {
		return nil, err
	}
	router := gin.Default()
	router.Use(corsMiddleware(cfg.CORS.AllowedOrigins))

	itemRepo := mongodb.NewItemRepository(db)
	packRepo := mongodb.NewPackRepository(db)
	checklistRepo := mongodb.NewChecklistRepository(db)
	categoryRepo := mongodb.NewCategoryRepository(db)
	launchScreenRepo := mongodb.NewLaunchScreenRepository(db)
	userRepo := mongodb.NewUserRepository(db)
	authIdentityRepo := mongodb.NewAuthIdentityRepository(db)
	refreshTokenRepo := mongodb.NewRefreshTokenRepository(db)
	passwordCredentialRepo := mongodb.NewUserPasswordCredentialRepository(db)
	uploadService := service.NewOSSUploadService(bucket, cfg.OSS.SignedURLTTLSeconds)
	tokenService := service.NewTokenService(cfg.Auth.AccessTokenSecret, time.Duration(cfg.Auth.AccessTokenTTLSeconds)*time.Second)
	categoryService := service.NewCategoryService(categoryRepo)
	launchScreenService := service.NewLaunchScreenService(launchScreenRepo, uploadService)
	itemService := service.NewItemService(itemRepo, uploadService, categoryService)
	packService := service.NewPackService(packRepo, itemRepo)
	checklistService := service.NewChecklistService(checklistRepo, itemRepo)
	authService := service.NewAuthService(userRepo, authIdentityRepo, refreshTokenRepo, passwordCredentialRepo, uploadService, verification, tokenService, cfg.Auth, cfg.Env)
	chatClient := llm.NewDeepSeekChatClient(cfg.Deepseek)
	recommendationAgent := agent.NewRecommendationPlannerAgent(chatClient)
	itemDraftExtractionAgent := agent.NewChatItemDraftExtractionAgent(chatClient)
	aiSuggestionService := service.NewAISuggestionService(itemService, categoryService, recommendationAgent, itemDraftExtractionAgent)

	registerAPIRoutes(router, itemService, packService, checklistService, categoryService, launchScreenService, aiSuggestionService, authService, tokenService, uploadService, cfg.Auth.ProfileUpload)

	return router, nil
}

func newPhoneVerificationService(cfg *config.Configuration) (service.PhoneVerificationService, error) {
	if cfg.Env == "dev" {
		return nil, nil
	}
	if cfg.Env != "prod" {
		return nil, fmt.Errorf("invalid config: env must be dev or prod")
	}
	return service.NewAliyunPhoneVerificationService(cfg.Auth.PhoneCode)
}

func registerAPIRoutes(router *gin.Engine, itemService service.ItemService, packService service.PackService, checklistService service.ChecklistService, categoryService service.CategoryService, launchScreenService service.LaunchScreenService, aiSuggestionService service.AISuggestionService, authService service.AuthService, tokenService service.TokenService, objectURLSigner service.ObjectURLSigner, profileLimits ...config.ProfileUploadConfig) {
	itemHandler := handler.NewItemHandler(itemService, categoryService, objectURLSigner)
	packHandler := handler.NewPackHandler(packService)
	checklistHandler := handler.NewChecklistHandler(checklistService)
	categoryHandler := handler.NewCategoryHandler(categoryService)
	aiHandler := handler.NewAIHandler(aiSuggestionService)
	launchScreenHandler := handler.NewLaunchScreenHandler(launchScreenService)
	authHandler := handler.NewAuthHandler(authService, objectURLSigner, profileLimits...)
	authMiddleware := handler.NewAuthMiddleware(tokenService, authService)

	router.GET("/api/v1/categories", categoryHandler.ListCategories)
	router.GET("/api/v1/app/launch-screen", launchScreenHandler.Get)

	authRoutes := router.Group("/api/v1/auth")
	authRoutes.POST("/phone/code", authHandler.SendPhoneCode)
	authRoutes.POST("/phone/login", authHandler.LoginWithPhone)
	authRoutes.POST("/phone/code/login", authHandler.LoginWithPhone)
	authRoutes.POST("/phone/password/login", authHandler.LoginWithPhonePassword)
	authRoutes.POST("/refresh", authHandler.Refresh)
	authRoutes.POST("/logout", authHandler.Logout)
	authRoutes.POST("/password/setup", authMiddleware.RequireAuth(), authHandler.SetupPassword)
	authRoutes.POST("/password/reset", authMiddleware.RequireAuth(), authHandler.ResetPassword)
	authRoutes.PUT("/password", authMiddleware.RequireAuth(), authHandler.ChangePassword)
	authRoutes.GET("/security", authMiddleware.RequireAuth(), authHandler.GetSecurity)
	authRoutes.DELETE("/me", authMiddleware.RequireAuth(), authHandler.DeleteMe)
	router.GET("/api/v1/me", authMiddleware.RequireAuth(), authHandler.Me)
	router.PATCH("/api/v1/me/profile", authMiddleware.RequireAuth(), authHandler.PatchMyProfile)
	router.PUT("/api/v1/me/profile", authMiddleware.RequireAuth(), authHandler.UpdateMyProfile)

	aiRoutes := router.Group("/api/v1/ai")
	aiRoutes.Use(authMiddleware.RequireAuth())
	aiRoutes.POST("/item-drafts", aiHandler.GenerateItemDrafts)
	aiRoutes.POST("/pack/item-recommendations", aiHandler.RecommendPackItems)

	itemRoutes := router.Group("/api/v1/item")
	itemRoutes.Use(authMiddleware.RequireAuth())
	itemRoutes.POST("", itemHandler.CreateItem)
	itemRoutes.POST("/batch", itemHandler.CreateItemsBatch)
	itemRoutes.GET("", itemHandler.ListItems)
	itemRoutes.GET("/:item_id", itemHandler.GetItem)
	itemRoutes.PUT("/:item_id", itemHandler.UpdateItem)
	itemRoutes.DELETE("/:item_id", itemHandler.DeleteItem)

	packRoutes := router.Group("/api/v1/pack")
	packRoutes.Use(authMiddleware.RequireAuth())
	packRoutes.POST("", packHandler.CreatePack)
	packRoutes.GET("", packHandler.ListPacks)
	packRoutes.GET("/:pack_id", packHandler.GetPack)
	packRoutes.PATCH("/:pack_id/profile", packHandler.UpdatePackProfile)
	packRoutes.POST("/:pack_id/items", packHandler.AddPackItems)
	packRoutes.DELETE("/:pack_id/items", packHandler.RemovePackItems)
	packRoutes.DELETE("/:pack_id", packHandler.DeletePack)

	checklistRoutes := router.Group("/api/v1/checklist")
	checklistRoutes.Use(authMiddleware.RequireAuth())
	checklistRoutes.POST("", checklistHandler.CreateChecklist)
	checklistRoutes.GET("", checklistHandler.ListChecklists)
	checklistRoutes.GET("/:checklist_id", checklistHandler.GetChecklist)
	checklistRoutes.PUT("/:checklist_id", checklistHandler.UpdateChecklist)
	checklistRoutes.POST("/:checklist_id/items", checklistHandler.AddChecklistLineItems)
	checklistRoutes.DELETE("/:checklist_id/items", checklistHandler.RemoveChecklistLineItems)
	checklistRoutes.PATCH("/:checklist_id/items/:line_item_id/status", checklistHandler.UpdateChecklistLineItemStatus)
	checklistRoutes.DELETE("/:checklist_id", checklistHandler.DeleteChecklist)
}

func corsMiddleware(origins []string) gin.HandlerFunc {
	allowedOrigins := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		allowedOrigins[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowedOrigins[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
