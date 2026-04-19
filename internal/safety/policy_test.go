package safety

import (
	"testing"

	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/portainer/models"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeLocalStacks(t *testing.T) {
	policy, err := NewPolicy(Config{SafeMode: true})
	assert.NoError(t, err)

	stacks := []models.LocalStack{
		{
			ID:   1,
			Name: "demo",
			Env: []models.LocalStackEnvVar{
				{Name: "DB_PASSWORD", Value: "super-secret"},
				{Name: "API_TOKEN", Value: "abc123"},
				{Name: "LOG_LEVEL", Value: "debug"},
			},
		},
	}

	redacted, note := policy.SanitizeLocalStacks(stacks)

	assert.Equal(t, "<redacted>", redacted[0].Env[0].Value)
	assert.Equal(t, "<redacted>", redacted[0].Env[1].Value)
	assert.Equal(t, "debug", redacted[0].Env[2].Value)
	assert.NotNil(t, note)
	assert.True(t, note.Redacted)
	assert.Equal(t, []string{"API_TOKEN", "DB_PASSWORD"}, note.RedactedKeys)
}

func TestSanitizeComposeContent(t *testing.T) {
	policy, err := NewPolicy(Config{SafeMode: true})
	assert.NoError(t, err)

	compose := `services:
  app:
    environment:
      DB_PASSWORD: super-secret
      LOG_LEVEL: debug
    secrets:
      - db_password
  worker:
    environment:
      - API_TOKEN=abc123
      - LOG_LEVEL=info
secrets:
  db_password:
    file: ./db_password.txt
`

	redacted, note, err := policy.SanitizeComposeContent(compose)
	assert.NoError(t, err)
	assert.NotNil(t, note)
	assert.Contains(t, redacted, "DB_PASSWORD: <redacted>")
	assert.Contains(t, redacted, "API_TOKEN=<redacted>")
	assert.Contains(t, redacted, "LOG_LEVEL: debug")
	assert.Contains(t, redacted, "LOG_LEVEL=info")
	assert.NotContains(t, redacted, "super-secret")
	assert.NotContains(t, redacted, "abc123")
	assert.NotContains(t, redacted, "./db_password.txt")
	assert.Contains(t, note.RedactedKeys, "DB_PASSWORD")
	assert.Contains(t, note.RedactedKeys, "API_TOKEN")
	assert.Contains(t, note.RedactedKeys, "secrets")
}

func TestCheckDockerProxy(t *testing.T) {
	policy, err := NewPolicy(Config{SafeMode: true})
	assert.NoError(t, err)

	assert.Nil(t, policy.CheckDockerProxy("GET", "/containers/json"))

	decision := policy.CheckDockerProxy("GET", "/containers/123/json")
	assert.NotNil(t, decision)
	assert.False(t, decision.Allowed)
	assert.Contains(t, decision.Message, "/containers/123/json")
	assert.True(t, decision.Note.Denied)
}

func TestSanitizeKubernetesJSON(t *testing.T) {
	policy, err := NewPolicy(Config{SafeMode: true})
	assert.NoError(t, err)

	body := []byte(`{"kind":"Secret","data":{"password":"c2VjcmV0"},"metadata":{"name":"demo"},"apiToken":"abc123"}`)

	redacted, note, err := policy.SanitizeKubernetesJSON(body)
	assert.NoError(t, err)
	assert.NotNil(t, note)
	assert.JSONEq(t, `{"kind":"Secret","data":"<redacted>","metadata":{"name":"demo"},"apiToken":"<redacted>"}`, string(redacted))
	assert.Contains(t, note.RedactedKeys, "Secret.data")
	assert.Contains(t, note.RedactedKeys, "apiToken")
}

func TestSafeModeDisabledReturnsOriginalValues(t *testing.T) {
	policy, err := NewPolicy(Config{SafeMode: false})
	assert.NoError(t, err)

	stacks := []models.LocalStack{
		{
			ID:   1,
			Name: "demo",
			Env: []models.LocalStackEnvVar{
				{Name: "DB_PASSWORD", Value: "super-secret"},
			},
		},
	}

	redactedStacks, note := policy.SanitizeLocalStacks(stacks)
	assert.Equal(t, stacks, redactedStacks)
	assert.Nil(t, note)

	compose := "services:\n  app:\n    environment:\n      DB_PASSWORD: super-secret\n"
	redactedCompose, composeNote, err := policy.SanitizeComposeContent(compose)
	assert.NoError(t, err)
	assert.Equal(t, compose, redactedCompose)
	assert.Nil(t, composeNote)
}
