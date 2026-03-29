package rbac

import (
	"path"
	"strings"

	"example.com/m/v2/internal/domain"
)

type Matcher struct{}

func NewMatcher() Matcher { return Matcher{} }

func (Matcher) RequiredScope(method string) string {
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "OPTIONS":
		return "read"
	case "POST", "PUT", "PATCH", "DELETE":
		return "write"
	default:
		return "memory"
	}
}

func (Matcher) Allowed(pathValue string, scopes []domain.Scope, accountType string, method string) bool {
	required := strings.ToLower(Matcher{}.RequiredScope(method))
	accountType = strings.ToLower(accountType)
	for _, scope := range scopes {
		if strings.ToLower(scope.Root) != accountType {
			continue
		}
		if strings.ToLower(scope.Scope) != required {
			continue
		}
		if globMatch(scope.Path, pathValue) {
			return true
		}
	}
	return false
}

func globMatch(pattern, value string) bool {
	pattern = normalize(pattern)
	value = normalize(value)
	ok, err := path.Match(pattern, value)
	if err == nil && ok {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}

func normalize(value string) string {
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		return "/" + value
	}
	return value
}
