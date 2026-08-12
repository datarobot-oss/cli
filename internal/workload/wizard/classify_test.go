// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wizard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The values below are shaped like the real thing but are not real
// credentials; the point is the shape, which is what the classifier reads.
func TestClassifyEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  EnvKind
		why   string
	}{
		// Issuer-specific shapes are the strongest signal there is.
		{"MODEL_PROVIDER", "sk-abcdefghijklmnopqrstuvwxyz012345", EnvSecret, "an OpenAI-style key is a secret whatever the variable is called"},
		{"ANTHROPIC", "sk-ant-api03-abcdefghijklmnopqrstuvwxyz", EnvSecret, ""},
		{"GH", "ghp_abcdefghijklmnopqrstuvwxyz0123456789", EnvSecret, ""},
		{"HF", "hf_abcdefghijklmnopqrstuvwxyz012345", EnvSecret, ""},
		{"AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE", EnvSecret, ""},
		{"SLACK", "xoxb-1234567890-abcdefghijkl", EnvSecret, ""},
		{"SIGNING", "-----BEGIN RSA PRIVATE KEY-----\nMIIE", EnvSecret, ""},
		{"SESSION", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc", EnvSecret, "a JWT is a bearer credential"},

		// A password inside a connection string, however innocuous the name.
		{"DATABASE_URL", "postgres://app:hunter2@db.internal:5432/prod", EnvSecret, "the password is in the value"},
		{"DATABASE_URL", "postgres://db.internal:5432/prod", EnvConfig, "no credentials, so nothing to hide"},

		// The name test, corroborated by the value.
		{"OPENAI_API_KEY", "abc123XYZdef456GHIjkl789", EnvSecret, ""},
		{"STRIPE_SECRET", "", EnvSecret, "empty here, but it is still the slot for a secret"},

		// The name test, contradicted by the value. This is what a
		// name-only classifier gets wrong.
		{"LOG_LEVEL_KEY", "debug", EnvConfig, "the value cannot be a secret"},
		{"DEBUG_TOKEN_ENABLED", "true", EnvConfig, ""},
		{"SSH_KEY_PATH", "/home/dev/.ssh/id_rsa", EnvConfig, "a path names a secret, it is not one"},
		{"CERT_FILE", "/etc/ssl/cert.pem", EnvConfig, ""},
		{"AUTH_URL", "https://auth.example.com/oauth", EnvConfig, ""},

		// Entropy, for the secrets nobody named helpfully.
		{"UPLOAD", "f3a9c2e17b40d85639ae0c71f2b84d5e", EnvSecret, "a hex digest is a random token"},
		{"SESSION_STORE", "Xk9mPq2vLw8nRt5yBc3zFh6jDg4sAe7U", EnvSecret, ""},

		// Ordinary settings that entropy must not convict.
		{"LOG_LEVEL", "debug", EnvConfig, ""},
		{"PORT", "8080", EnvConfig, ""},
		{"TIMEOUT_SECONDS", "30.5", EnvConfig, "a number is not a token"},
		{"REGION", "us-east-1", EnvConfig, ""},
		{"STATIC_ROOT", "/var/www/static/assets/images", EnvConfig, "a long path is structured, not random"},
		{"GREETING", "hello there how are you today", EnvConfig, "prose is not random"},
		{"FEATURE_FLAGS", "alpha,beta,gamma", EnvConfig, ""},

		// Local-only, which is neither config nor secret.
		{"VITE_DEV_PORT", "3000", EnvLocal, "a build-time frontend variable"},
		{"NEXT_PUBLIC_API_URL", "https://api.example.com", EnvLocal, ""},
		{"REDIS_URL", "redis://localhost:6379", EnvLocal, "points at this machine"},
		{"API_BASE", "http://127.0.0.1:8000", EnvLocal, ""},
		{"DEV_MODE", "on", EnvLocal, ""},

		// A frontend prefix wins over a name that looks secret, because those
		// variables are public by construction.
		{"VITE_API_KEY", "abc123XYZdef456GHI", EnvLocal, "a public build-time variable is already not a secret"},
	}

	for _, test := range tests {
		t.Run(test.name+"="+truncate(test.value), func(t *testing.T) {
			got := ClassifyEnv(test.name, test.value)

			assert.Equalf(t, test.want, got.Kind, "%s (reason given: %s)", test.why, got.Reason)
			assert.NotEmpty(t, got.Reason, "every verdict explains itself")
			assert.Equal(t, test.name, got.Name)
		})
	}
}

// Whatever the verdict, the value never travels with it.
func TestClassifyEnv_CarriesNoValue(t *testing.T) {
	got := ClassifyEnv("OPENAI_API_KEY", "sk-abcdefghijklmnopqrstuvwxyz012345")

	assert.NotContains(t, got.Reason, "sk-")
	assert.NotContains(t, got.Name, "sk-")
}

func TestEnvKind_String(t *testing.T) {
	assert.Equal(t, "config", EnvConfig.String())
	assert.Equal(t, "secret", EnvSecret.String())
	assert.Equal(t, "local only", EnvLocal.String())
}

func truncate(value string) string {
	const limit = 24
	if len(value) <= limit {
		return value
	}

	return value[:limit] + "…"
}

// A URL is judged by its parts. Structure is not randomness, but an opaque
// segment inside a path is how webhook URLs carry their secret.
func TestClassifyEnv_URLsAreJudgedByTheirParts(t *testing.T) {
	tests := map[string]struct {
		name, value string
		want        EnvKind
	}{
		"plain connection string": {"DATABASE_URL", "postgres://db.internal:5432/prod", EnvConfig},
		"long ordinary path":      {"ASSET_BASE", "https://cdn.example.com/static/images/products/large", EnvConfig},
		"query with plain values": {"SEARCH_URL", "https://api.example.com/v1/search?limit=100&sort=name", EnvConfig},
		// A real provider host here would be blocked by secret scanning
		// on push, so the host is fictional. The rule under test is
		// structural and does not look at the host.
		"webhook with a token":    {"NOTIFY_WEBHOOK", "https://hooks.example.com/services/T00000000/B00000000/XyZ9aBc3DeF6gHi2JkL5mNoP", EnvSecret},
		"token in a query string": {"CALLBACK", "https://api.example.com/hook?token=Xk9mPq2vLw8nRt5yBc3zFh6j", EnvSecret},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := ClassifyEnv(test.name, test.value)
			assert.Equalf(t, test.want, got.Kind, "reason given: %s", got.Reason)
		})
	}
}
