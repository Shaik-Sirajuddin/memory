package oauth

import (
	"context"
	"errors"
	"strings"

	"example.com/m/v2/internal/config"
	"example.com/m/v2/internal/domain"
)

type Resolver struct {
	cfg config.Config
}

func NewResolver(cfg config.Config) Resolver {
	return Resolver{cfg: cfg}
}

func (r Resolver) Resolve(ctx context.Context, provider, token string) (domain.OAuthProfile, error) {
	_ = ctx
	provider = strings.ToLower(provider)
	if provider != "google" && provider != "github" {
		return domain.OAuthProfile{}, errors.New("unsupported provider")
	}
	if strings.HasPrefix(token, "mock:") {
		parts := strings.Split(strings.TrimPrefix(token, "mock:"), ":")
		if len(parts) == 1 {
			return domain.OAuthProfile{Provider: provider, ProviderUID: parts[0], Email: parts[0]}, nil
		}
		return domain.OAuthProfile{Provider: provider, ProviderUID: parts[1], Email: parts[0]}, nil
	}
	if strings.Contains(token, "@") {
		return domain.OAuthProfile{Provider: provider, ProviderUID: token, Email: token}, nil
	}
	return domain.OAuthProfile{}, errors.New("oauth token resolution not configured for live provider exchange")
}
