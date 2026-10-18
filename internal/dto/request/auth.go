package request

import "io"

// SendPhoneCodeInput defines the input for sending a phone login code.
type SendPhoneCodeInput struct {
	Phone string `json:"phone" binding:"required"`
}

// PhoneLoginInput defines the input for phone login.
type PhoneLoginInput struct {
	Phone string `json:"phone" binding:"required"`
	Code  string `json:"code" binding:"required,len=6,numeric"`
}

// PhonePasswordLoginInput defines the input for phone password login.
type PhonePasswordLoginInput struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

// RefreshTokenInput defines the input for refreshing auth tokens.
type RefreshTokenInput struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutInput defines the input for logging out.
type LogoutInput struct {
	RefreshToken string `json:"refresh_token"`
}

// SetupPasswordInput defines the input for setting up a login password.
type SetupPasswordInput struct {
	UserID   string `json:"-"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

// ChangePasswordInput defines the input for changing a login password.
type ChangePasswordInput struct {
	UserID      string `json:"-"`
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ResetPasswordInput defines the HTTP input for resetting a password with a phone code.
type ResetPasswordInput struct {
	Phone       string `json:"phone" binding:"required"`
	Code        string `json:"code" binding:"required,len=6,numeric"`
	NewPassword string `json:"new_password" binding:"required"`
}

// UpdateMyProfileInput defines the input for updating the current user profile.
type UpdateMyProfileInput struct {
	Username string
	Gender   string
	Birthday string
	File     io.ReadSeeker
	FileName string
}
