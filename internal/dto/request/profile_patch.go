package request

// ProfilePatchRequest contains optional multipart text fields for profile updates.
type ProfilePatchRequest struct {
	Username *string `form:"username"`
	Gender   *string `form:"gender"`
	Birthday *string `form:"birthday"`
}
