package category

import "testing"

func TestIsLikelyValidOpenRouterAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected bool
	}{
		{name: "valid key", apiKey: "sk-or-v1-abc123", expected: true},
		{name: "valid key with whitespace", apiKey: "  sk-or-v1-abc123 \n", expected: true},
		{name: "empty key", apiKey: "", expected: false},
		{name: "wrong prefix", apiKey: "sk-test-abc123", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLikelyValidOpenRouterAPIKey(tt.apiKey); got != tt.expected {
				t.Fatalf("IsLikelyValidOpenRouterAPIKey() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
