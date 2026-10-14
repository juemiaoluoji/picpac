package handler

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"pack_mate/internal/dto/request"
	"pack_mate/internal/dto/response"
	"pack_mate/internal/service"
)

// ResetPassword handles authenticated phone-code password resets.
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var input request.ResetPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondPhoneInputError(c, err)
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), service.ResetPasswordInput{UserID: userID, Phone: input.Phone, Code: input.Code, NewPassword: input.NewPassword}); err != nil {
		respondAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, response.ResetPasswordResponse{Reset: true})
}
