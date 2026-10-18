package handler

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
	"pack_mate/internal/dto/request"
	"pack_mate/internal/service"
)

// PatchMyProfile handles partial updates of the authenticated user's profile.
func (h *AuthHandler) PatchMyProfile(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	input, cleanup, err := h.parseProfileForm(c)
	defer cleanup()
	if err != nil {
		respondProfileInputError(c, err)
		return
	}
	user, err := h.svc.PatchMyProfile(c.Request.Context(), userID, input)
	if err != nil {
		respondAuthError(c, err)
		return
	}
	result, err := h.buildUserResponse(c.Request.Context(), user)
	if err != nil {
		respondAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *AuthHandler) parseProfileForm(c *gin.Context) (service.ProfilePatchInput, func(), error) {
	input := service.ProfilePatchInput{}
	var file multipart.File
	cleanup := func() {
		if file != nil {
			_ = file.Close()
		}
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}
	limit := h.profileUpload.WithDefaults().MaxBytes
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit+(1<<20))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		return input, cleanup, err
	}
	form := c.Request.MultipartForm
	dto := request.ProfilePatchRequest{}
	fields := map[string]**string{"username": &dto.Username, "gender": &dto.Gender, "birthday": &dto.Birthday}
	for name, values := range form.Value {
		target, ok := fields[name]
		if !ok || len(values) != 1 {
			return input, cleanup, fmt.Errorf("invalid input")
		}
		value := values[0]
		*target = &value
	}
	input.Username = dto.Username
	input.Gender = dto.Gender
	input.Birthday = dto.Birthday
	for name, files := range form.File {
		if name != "avatar" || len(files) != 1 {
			return input, cleanup, fmt.Errorf("invalid input")
		}
		if files[0].Size > limit {
			return input, cleanup, service.ErrAvatarTooLarge
		}
		var err error
		file, err = files[0].Open()
		if err != nil {
			return input, cleanup, err
		}
		input.File = file
		input.FileName = files[0].Filename
	}
	return input, cleanup, nil
}

func respondProfileInputError(c *gin.Context, err error) {
	var sizeErr *http.MaxBytesError
	if errors.As(err, &sizeErr) || errors.Is(err, service.ErrAvatarTooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "avatar exceeds upload limits"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
}
