package category

import "strings"

func IsLikelyValidOpenRouterAPIKey(apiKey string) bool {
	apiKey = strings.TrimSpace(apiKey)
	return apiKey != "" && strings.HasPrefix(apiKey, "sk-or-")
}
