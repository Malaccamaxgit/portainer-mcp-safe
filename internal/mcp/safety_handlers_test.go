package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Malaccamaxgit/portainer-mcp-safe/internal/safety"
	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/portainer/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHandleGetLocalStacksSafeMode(t *testing.T) {
	mockClient := &MockPortainerClient{}
	mockClient.On("GetLocalStacks").Return([]models.LocalStack{
		{
			ID:   1,
			Name: "demo",
			Env: []models.LocalStackEnvVar{
				{Name: "DB_PASSWORD", Value: "super-secret"},
				{Name: "LOG_LEVEL", Value: "debug"},
			},
		},
	}, nil)

	policy, err := safety.NewPolicy(safety.Config{SafeMode: true})
	assert.NoError(t, err)

	server := &PortainerMCPServer{
		cli:    mockClient,
		policy: policy,
	}

	result, err := server.HandleGetLocalStacks()(context.Background(), mcp.CallToolRequest{})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Len(t, result.Content, 2)

	payload := result.Content[0].(mcp.TextContent).Text
	assert.JSONEq(t, `[{"id":1,"name":"demo","type":"","status":"","endpoint_id":0,"created_at":"","env":[{"name":"DB_PASSWORD","value":"<redacted>"},{"name":"LOG_LEVEL","value":"debug"}]}]`, payload)

	safetyNote := result.Content[1].(mcp.TextContent).Text
	assert.Contains(t, safetyNote, `"_safety"`)
	assert.Contains(t, safetyNote, `"DB_PASSWORD"`)

	mockClient.AssertExpectations(t)
}

func TestHandleDockerProxySafeModeDeniesSensitivePath(t *testing.T) {
	policy, err := safety.NewPolicy(safety.Config{SafeMode: true})
	assert.NoError(t, err)

	server := &PortainerMCPServer{
		policy: policy,
	}

	request := CreateMCPRequest(map[string]any{
		"environmentId": float64(1),
		"dockerAPIPath": "/containers/123/json",
		"method":        "GET",
	})

	result, err := server.HandleDockerProxy()(context.Background(), request)
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Len(t, result.Content, 2)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "denied by safe_mode")
	assert.Contains(t, result.Content[1].(mcp.TextContent).Text, `"_safety"`)
}

func TestHandleKubernetesProxySafeModeRedactsResponse(t *testing.T) {
	mockClient := &MockPortainerClient{}
	mockClient.On("ProxyKubernetesRequest", mock.MatchedBy(createKubernetesProxyMatcher(1, "GET", "/api/v1/pods"))).
		Return(createMockHttpResponse(200, `{"kind":"PodList","items":[{"metadata":{"name":"demo"},"apiToken":"abc123"}]}`), nil)

	policy, err := safety.NewPolicy(safety.Config{SafeMode: true})
	assert.NoError(t, err)

	server := &PortainerMCPServer{
		cli:    mockClient,
		policy: policy,
	}

	request := CreateMCPRequest(map[string]any{
		"environmentId":     float64(1),
		"kubernetesAPIPath": "/api/v1/pods",
		"method":            "GET",
	})

	result, err := server.HandleKubernetesProxy()(context.Background(), request)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Len(t, result.Content, 2)
	assert.JSONEq(t, `{"kind":"PodList","items":[{"metadata":{"name":"demo"},"apiToken":"<redacted>"}]}`, result.Content[0].(mcp.TextContent).Text)

	var safetyEnvelope map[string]any
	err = json.Unmarshal([]byte(result.Content[1].(mcp.TextContent).Text), &safetyEnvelope)
	assert.NoError(t, err)
	assert.Contains(t, result.Content[1].(mcp.TextContent).Text, `"apiToken"`)

	mockClient.AssertExpectations(t)
}

func createKubernetesProxyMatcher(environmentID int, method string, path string) func(models.KubernetesProxyRequestOptions) bool {
	return func(opts models.KubernetesProxyRequestOptions) bool {
		return opts.EnvironmentID == environmentID && opts.Method == method && opts.Path == path
	}
}
