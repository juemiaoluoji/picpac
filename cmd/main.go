package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pack_mate/internal/config"
	mongodb "pack_mate/internal/repository/mongodb"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

func main() {
	appCfg, err := loadConfig()
	if err != nil {
		panic(err)
	}

	bucket, err := newOSSBucket(appCfg)
	if err != nil {
		panic(err)
	}

	mongoConn, err := mongodb.New(appCfg.Mongo)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = mongoConn.Close(context.Background())
	}()
	categorySeeds, err := loadCategorySeeds()
	if err != nil {
		panic(err)
	}
	if err := mongodb.SeedCategories(context.Background(), mongoConn.Database, categorySeeds.Categories); err != nil {
		panic(err)
	}

	port := strings.TrimSpace(appCfg.Server.Port)
	if port == "" {
		port = "8080"
	}

	router, err := newRouter(appCfg, bucket, mongoConn.Database)
	if err != nil {
		panic(err)
	}

	if err := router.Run(":" + port); err != nil {
		panic(err)
	}
}

func loadConfig() (*config.Configuration, error) {
	candidates := []string{}
	if path := strings.TrimSpace(os.Getenv("PICPAC_CONFIG")); path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, "config.json", filepath.Join("..", "config.json"))

	if executablePath, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executablePath)
		candidates = append(candidates,
			filepath.Join(executableDir, "config.json"),
			filepath.Join(executableDir, "..", "config.json"),
		)
	}

	var lastErr error
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		cleanPath := filepath.Clean(candidate)
		if _, ok := seen[cleanPath]; ok {
			continue
		}
		seen[cleanPath] = struct{}{}

		cfg, err := config.Load(cleanPath)
		if err == nil {
			return cfg, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		lastErr = err
	}

	return nil, fmt.Errorf("load config.json: %w", lastErr)
}

func loadCategorySeeds() (*config.CategorySeedFile, error) {
	candidates := []string{}
	if path := strings.TrimSpace(os.Getenv("PICPAC_CATEGORY_CONFIG")); path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, "category.json", filepath.Join("..", "category.json"))

	if executablePath, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executablePath)
		candidates = append(candidates,
			filepath.Join(executableDir, "category.json"),
			filepath.Join(executableDir, "..", "category.json"),
		)
	}

	var lastErr error
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		cleanPath := filepath.Clean(candidate)
		if _, ok := seen[cleanPath]; ok {
			continue
		}
		seen[cleanPath] = struct{}{}

		seeds, err := config.LoadCategorySeeds(cleanPath)
		if err == nil {
			return seeds, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		lastErr = err
	}

	return nil, fmt.Errorf("load category.json: %w", lastErr)
}

func newOSSBucket(cfg *config.Configuration) (*oss.Bucket, error) {
	endpoint := strings.TrimSpace(cfg.OSS.Endpoint)
	bucketName := strings.TrimSpace(cfg.OSS.BucketName)
	accessKeyID := strings.TrimSpace(cfg.OSS.AccessKeyID)
	accessKeySecret := strings.TrimSpace(cfg.OSS.AccessKeySecret)
	publicBaseURL := strings.TrimRight(strings.TrimSpace(cfg.OSS.PublicBaseURL), "/")

	switch {
	case endpoint == "":
		return nil, errors.New("missing OSS_ENDPOINT")
	case bucketName == "":
		return nil, errors.New("missing OSS_BUCKET_NAME")
	case accessKeyID == "":
		return nil, errors.New("missing OSS_ACCESS_KEY_ID")
	case accessKeySecret == "":
		return nil, errors.New("missing OSS_ACCESS_KEY_SECRET")
	case publicBaseURL == "":
		return nil, errors.New("missing OSS_PUBLIC_BASE_URL")
	}

	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("create OSS client: %w", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("create OSS bucket: %w", err)
	}

	return bucket, nil
}
