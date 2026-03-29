package domain

import "time"

type Account struct {
	UID          string    `json:"uid"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	AccountType  string    `json:"account_type"`
	Organization string    `json:"organization"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type OAuthLink struct {
	UID          string    `json:"uid"`
	AccountUID   string    `json:"account_uid"`
	Provider     string    `json:"provider"`
	ProviderUID  string    `json:"provider_uid"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type ServiceAccount struct {
	UID        string     `json:"uid"`
	AccountUID string     `json:"account_uid"`
	Name       string     `json:"name"`
	AccessKey  string     `json:"access_key"`
	SecretKey  string     `json:"-"`
	ExpiryDate *time.Time `json:"expiry_date,omitempty"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Scope struct {
	UID               string    `json:"uid"`
	ServiceAccountUID string    `json:"service_account_uid"`
	Root              string    `json:"root"`
	Path              string    `json:"path"`
	Scope             string    `json:"scope"`
	CreatedAt         time.Time `json:"created_at"`
}

type OAuthProfile struct {
	Provider     string
	ProviderUID  string
	Email        string
	AccessToken  string
	RefreshToken string
}

type OAuthRequest struct {
	Provider string `json:"provider" validate:"required,oneof=google github"`
	Token    string `json:"token" validate:"required"`
}

type SignupRequest struct {
	Email    string `json:"email" validate:"required_without=Provider,omitempty,email"`
	Password string `json:"password" validate:"required_without=Provider,omitempty,min=8"`
	Provider string `json:"provider" validate:"omitempty,oneof=google github"`
	Token    string `json:"token" validate:"required_with=Provider,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required_without=Provider,omitempty,email"`
	Password string `json:"password" validate:"required_without=Provider,omitempty"`
	Provider string `json:"provider" validate:"omitempty,oneof=google github"`
	Token    string `json:"token" validate:"required_with=Provider,omitempty"`
}

type ServiceAccountCreateRequest struct {
	Name       string  `json:"name" validate:"required"`
	SecretKey  string  `json:"secret_key"`
	ExpiryDate string  `json:"expiry_date"`
	Scopes     []Scope `json:"scopes" validate:"required,min=1,dive"`
}

type ServiceAccountUpdateRequest struct {
	UID        string  `json:"uid" validate:"required"`
	Name       string  `json:"name" validate:"required"`
	SecretKey  string  `json:"secret_key"`
	ExpiryDate string  `json:"expiry_date"`
	IsActive   *bool   `json:"is_active"`
	Scopes     []Scope `json:"scopes" validate:"required,min=1,dive"`
}

type InitiateAuthRequest struct {
	ServiceAccountToken string `json:"service_account_token" validate:"required"`
}

type AuthenticateRequest struct {
	Token         string `json:"token" validate:"required"`
	SignedPayload string `json:"signedPayload"`
}

type Session struct {
	Token      string
	AccountUID string
	ExpiresAt  time.Time
}

type JWTClaims struct {
	AccountUID        string  `json:"account_uid"`
	ServiceAccountUID string  `json:"service_account_uid"`
	AccountType       string  `json:"account_type"`
	Scopes            []Scope `json:"scopes"`
	jti               string
}
