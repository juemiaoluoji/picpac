package handler

import (
	"net/http"

	"pack_mate/internal/domain"
	"pack_mate/internal/dto/response"
	"pack_mate/internal/service"

	"github.com/gin-gonic/gin"
)

// LaunchScreenHandler handles public launch screen requests.
type LaunchScreenHandler struct {
	svc service.LaunchScreenService
}

// NewLaunchScreenHandler creates a launch screen handler.
func NewLaunchScreenHandler(svc service.LaunchScreenService) *LaunchScreenHandler {
	return &LaunchScreenHandler{svc: svc}
}

// Get returns the current launch screen configuration.
func (h *LaunchScreenHandler) Get(c *gin.Context) {
	launchScreen, err := h.svc.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Cache-Control", "no-cache")
	c.JSON(http.StatusOK, buildLaunchScreenResponse(launchScreen))
}

func buildLaunchScreenResponse(config *domain.LaunchScreen) response.LaunchScreenResponse {
	if config == nil || !config.Enabled {
		return response.LaunchScreenResponse{SkipEnabled: true}
	}
	return response.LaunchScreenResponse{
		Enabled:     true,
		Revision:    config.Revision,
		Type:        string(config.Type),
		MediaURL:    config.MediaKey,
		PosterURL:   config.PosterKey,
		DurationMS:  config.DurationMS,
		SkipEnabled: config.SkipEnabled,
	}
}
