package response

// SendPhoneCodeResponse defines the API response for sending a phone code.
type SendPhoneCodeResponse struct {
	Sent bool `json:"sent"`
}

// AuthResponse defines the API response for successful authentication.
type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         UserResponse `json:"user"`
}

// RefreshAccessTokenResponse defines the API response for refreshing an access token.
type RefreshAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// UserResponse defines the API response for a user.
type UserResponse struct {
	ID      string              `json:"id"`
	Profile UserProfileResponse `json:"profile"`
	Status  string              `json:"status"`
}

// UserProfileResponse defines the API response for a user profile.
type UserProfileResponse struct {
	Username        string `json:"username"`
	Gender          string `json:"gender"`
	Birthday        string `json:"birthday"`
	AvatarURL       string `json:"avatar_url"`
	AvatarSourceURL string `json:"avatar_source_url"`
}

// LogoutResponse defines the API response for logout.
type LogoutResponse struct {
	LoggedOut bool `json:"logged_out"`
}

// SetupPasswordResponse defines the API response for setting up a login password.
type SetupPasswordResponse struct {
	Setup bool `json:"setup"`
}

// ChangePasswordResponse defines the API response for changing a login password.
type ChangePasswordResponse struct {
	Changed bool `json:"changed"`
}

// AuthSecurityResponse defines the API response for account security status.
type AuthSecurityResponse struct {
	Phone         string `json:"phone"`
	PasswordSetup bool   `json:"password_setup"`
}

// DeleteMeResponse defines the API response for deleting the current account.
type DeleteMeResponse struct {
	Deleted bool `json:"deleted"`
}

// ResetPasswordResponse defines the response for a completed password reset.
type ResetPasswordResponse struct {
	Reset bool `json:"reset"`
}
