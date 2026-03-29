package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"example.com/m/v2/internal/crypto"
	"example.com/m/v2/internal/domain"
	"example.com/m/v2/internal/util"
)

type PasswordHasher struct {
	cost int
}

func NewPasswordHasher(_ int) PasswordHasher {
	return PasswordHasher{}
}

func (h PasswordHasher) Hash(password string) (string, error) {
	sum := sha256.Sum256([]byte(password))
	return base64.RawStdEncoding.EncodeToString(sum[:]), nil
}

func (h PasswordHasher) Compare(hash, password string) error {
	decoded, err := base64.RawStdEncoding.DecodeString(hash)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(password))
	if !hmac.Equal(decoded, sum[:]) {
		return errors.New("invalid password")
	}
	return nil
}

type SecretCipher struct {
	cipher crypto.Cipher
}

func NewSecretCipher(key []byte) SecretCipher {
	return SecretCipher{cipher: crypto.NewCipher(key)}
}

func (s SecretCipher) Encrypt(value string) (string, error) { return s.cipher.Encrypt(value) }
func (s SecretCipher) Decrypt(value string) (string, error) { return s.cipher.Decrypt(value) }

type JWTManager struct {
	secrets [][]byte
	ttl     time.Duration
}

func NewJWTManager(secrets [][]byte, ttl time.Duration) JWTManager {
	if len(secrets) == 0 {
		secrets = [][]byte{[]byte("development-secret")}
	}
	return JWTManager{secrets: secrets, ttl: ttl}
}

type JWTClaims struct {
	ID                string         `json:"jti"`
	AccountUID        string         `json:"account_uid"`
	ServiceAccountUID string         `json:"service_account_uid"`
	AccountType       string         `json:"account_type"`
	Scopes            []domain.Scope `json:"scopes"`
	IssuedAt          int64          `json:"iat"`
	ExpiresAt         int64          `json:"exp"`
}

func (m JWTManager) Sign(_ context.Context, claims JWTClaims) (string, error) {
	if claims.ID == "" {
		claims.ID = util.UUID()
	}
	now := time.Now().UTC()
	claims.IssuedAt = now.Unix()
	claims.ExpiresAt = now.Add(m.ttl).Unix()
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	hRaw, _ := json.Marshal(header)
	cRaw, _ := json.Marshal(claims)
	head := base64.RawURLEncoding.EncodeToString(hRaw)
	body := base64.RawURLEncoding.EncodeToString(cRaw)
	signingInput := head + "." + body
	mac := hmac.New(sha256.New, m.secrets[0])
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig, nil
}

func (m JWTManager) Parse(tokenStr string) (JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return JWTClaims{}, errors.New("invalid token")
	}
	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return JWTClaims{}, err
	}

	var claims JWTClaims
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return JWTClaims{}, err
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return JWTClaims{}, err
	}
	if claims.ExpiresAt > 0 && time.Now().UTC().Unix() > claims.ExpiresAt {
		return JWTClaims{}, errors.New("token expired")
	}
	for _, secret := range m.secrets {
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(signingInput))
		if hmac.Equal(sig, mac.Sum(nil)) {
			return claims, nil
		}
	}
	return JWTClaims{}, errors.New("invalid signature")
}

type SigningTokenState struct {
	ServiceAccountUID string `json:"service_account_uid"`
	IssuedAt          int64  `json:"iat"`
}

type SessionState struct {
	AccountUID string `json:"account_uid"`
}

type JWTAuthState struct {
	ServiceAccountUID string         `json:"service_account_uid"`
	AccountUID        string         `json:"account_uid"`
	Scopes            []domain.Scope `json:"scopes"`
}

func HMACSHA256Hex(secret, message []byte) string {
	return crypto.HMACSHA256Hex(secret, message)
}

func VerifyHMAC(secret, message []byte, provided string) bool {
	return strings.EqualFold(HMACSHA256Hex(secret, message), strings.TrimSpace(provided))
}

func EncodeJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
