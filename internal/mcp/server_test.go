package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/toolgen"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPortainerMCPServer(t *testing.T) {
	// Define paths to test data files
	validToolsPath := "testdata/valid_tools.yaml"
	invalidToolsPath := "testdata/invalid_tools.yaml"

	tests := []struct {
		name          string
		serverURL     string
		token         string
		toolsPath     string
		mockSetup     func(*MockPortainerClient)
		expectError   bool
		errorContains string
	}{
		{
			name:      "successful initialization with supported version",
			serverURL: "https://portainer.example.com",
			token:     "valid-token",
			toolsPath: validToolsPath,
			mockSetup: func(m *MockPortainerClient) {
				m.On("GetVersion").Return(SupportedPortainerVersion, nil)
			},
			expectError: false,
		},
		{
			name:          "invalid tools path",
			serverURL:     "https://portainer.example.com",
			token:         "valid-token",
			toolsPath:     "testdata/nonexistent.yaml",
			mockSetup:     func(m *MockPortainerClient) {},
			expectError:   true,
			errorContains: "failed to load tools",
		},
		{
			name:          "invalid tools version",
			serverURL:     "https://portainer.example.com",
			token:         "valid-token",
			toolsPath:     invalidToolsPath,
			mockSetup:     func(m *MockPortainerClient) {},
			expectError:   true,
			errorContains: "invalid version in tools.yaml",
		},
		{
			name:      "API communication error",
			serverURL: "https://portainer.example.com",
			token:     "valid-token",
			toolsPath: validToolsPath,
			mockSetup: func(m *MockPortainerClient) {
				m.On("GetVersion").Return("", errors.New("connection error"))
			},
			expectError:   true,
			errorContains: "failed to get Portainer server version",
		},
		{
			name:      "unsupported Portainer version",
			serverURL: "https://portainer.example.com",
			token:     "valid-token",
			toolsPath: validToolsPath,
			mockSetup: func(m *MockPortainerClient) {
				m.On("GetVersion").Return("2.0.0", nil)
			},
			expectError:   true,
			errorContains: "unsupported Portainer server version",
		},
		{
			name:      "unsupported version with disabled version check",
			serverURL: "https://portainer.example.com",
			token:     "valid-token",
			toolsPath: validToolsPath,
			mockSetup: func(m *MockPortainerClient) {
				// No GetVersion call expected when version check is disabled
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create and configure the mock client
			mockClient := new(MockPortainerClient)
			tt.mockSetup(mockClient)

			// Create server with mock client using the WithClient option
			var options []ServerOption
			options = append(options, WithClient(mockClient))

			// Add WithDisableVersionCheck for the specific test case
			if tt.name == "unsupported version with disabled version check" {
				options = append(options, WithDisableVersionCheck(true))
			}

			server, err := NewPortainerMCPServer(
				tt.serverURL,
				tt.token,
				tt.toolsPath,
				options...,
			)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, server)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, server)
				assert.NotNil(t, server.srv)
				assert.NotNil(t, server.cli)
				assert.NotNil(t, server.tools)
				assert.NotNil(t, server.toolDefinitions)
			}

			// Verify that all expected methods were called
			mockClient.AssertExpectations(t)
		})
	}
}

func TestAddToolIfExists(t *testing.T) {
	tests := []struct {
		name               string
		tools              map[string]mcp.Tool
		toolDefinitions    map[string]toolgen.ToolDefinition
		toolName           string
		businessEdition    bool
		wantRegisteredTool bool
	}{
		{
			name: "existing tool",
			tools: map[string]mcp.Tool{
				"test_tool": {
					Name:        "test_tool",
					Description: "Test tool description",
					InputSchema: mcp.ToolInputSchema{
						Properties: map[string]any{},
					},
				},
			},
			toolDefinitions: map[string]toolgen.ToolDefinition{
				"test_tool": {
					Name: "test_tool",
				},
			},
			toolName:           "test_tool",
			wantRegisteredTool: true,
		},
		{
			name: "non-existing tool",
			tools: map[string]mcp.Tool{
				"test_tool": {
					Name:        "test_tool",
					Description: "Test tool description",
					InputSchema: mcp.ToolInputSchema{
						Properties: map[string]any{},
					},
				},
			},
			toolDefinitions:    map[string]toolgen.ToolDefinition{},
			toolName:           "nonexistent_tool",
			wantRegisteredTool: false,
		},
		{
			name: "business edition tool skipped in community mode",
			tools: map[string]mcp.Tool{
				"test_tool": {
					Name:        "test_tool",
					Description: "Test tool description",
					InputSchema: mcp.ToolInputSchema{
						Properties: map[string]any{},
					},
				},
			},
			toolDefinitions: map[string]toolgen.ToolDefinition{
				"test_tool": {
					Name:                    "test_tool",
					RequiresBusinessEdition: true,
				},
			},
			toolName:           "test_tool",
			businessEdition:    false,
			wantRegisteredTool: false,
		},
		{
			name: "business edition tool registered when enabled",
			tools: map[string]mcp.Tool{
				"test_tool": {
					Name:        "test_tool",
					Description: "Test tool description",
					InputSchema: mcp.ToolInputSchema{
						Properties: map[string]any{},
					},
				},
			},
			toolDefinitions: map[string]toolgen.ToolDefinition{
				"test_tool": {
					Name:                    "test_tool",
					RequiresBusinessEdition: true,
				},
			},
			toolName:           "test_tool",
			businessEdition:    true,
			wantRegisteredTool: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create server with test tools
			mcpServer := server.NewMCPServer(
				"Test Server",
				"1.0.0",
				server.WithResourceCapabilities(true, true),
				server.WithLogging(),
			)
			server := &PortainerMCPServer{
				tools:           tt.tools,
				toolDefinitions: tt.toolDefinitions,
				srv:             mcpServer,
				businessEdition: tt.businessEdition,
			}

			// Create a handler function
			handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{}, nil
			}

			// Call addToolIfExists
			server.addToolIfExists(tt.toolName, handler)

			// Verify if the tool exists in the tools map
			_, toolRegistered := server.registeredTools[tt.toolName]
			assert.Equal(t, tt.wantRegisteredTool, toolRegistered)
		})
	}
}

func TestEditionAwareToolRegistrationCounts(t *testing.T) {
	tests := []struct {
		name               string
		options            []ServerOption
		wantRegisteredTool int
		wantPresent        []string
		wantAbsent         []string
	}{
		{
			name:               "community edition registers ce subset",
			options:            []ServerOption{WithDisableVersionCheck(true)},
			wantRegisteredTool: 21,
			wantPresent:        []string{ToolListLocalStacks, ToolListEnvironments, ToolDockerProxy},
			wantAbsent:         []string{ToolListStacks, ToolListAccessGroups, ToolListEnvironmentGroups},
		},
		{
			name:               "business edition registers all tools",
			options:            []ServerOption{WithDisableVersionCheck(true), WithBusinessEdition(true)},
			wantRegisteredTool: 39,
			wantPresent:        []string{ToolListLocalStacks, ToolListStacks, ToolListAccessGroups, ToolListEnvironmentGroups},
		},
		{
			name:               "community edition read only registers intersection",
			options:            []ServerOption{WithDisableVersionCheck(true), WithReadOnly(true)},
			wantRegisteredTool: 10,
			wantPresent:        []string{ToolListLocalStacks, ToolGetLocalStackFile, ToolDockerProxy},
			wantAbsent:         []string{ToolCreateLocalStack, ToolListStacks, ToolListAccessGroups},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockPortainerClient)
			options := append([]ServerOption{WithClient(mockClient)}, tt.options...)
			server, err := NewPortainerMCPServer(
				"https://portainer.example.com",
				"valid-token",
				filepath.Join("..", "tooldef", "tools.yaml"),
				options...,
			)
			require.NoError(t, err)

			registerAllFeatures(server)

			require.Len(t, server.registeredTools, tt.wantRegisteredTool)
			for _, toolName := range tt.wantPresent {
				_, exists := server.registeredTools[toolName]
				assert.True(t, exists, "expected tool %s to be registered", toolName)
			}
			for _, toolName := range tt.wantAbsent {
				_, exists := server.registeredTools[toolName]
				assert.False(t, exists, "expected tool %s to be skipped", toolName)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func registerAllFeatures(server *PortainerMCPServer) {
	server.AddEnvironmentFeatures()
	server.AddEnvironmentGroupFeatures()
	server.AddTagFeatures()
	server.AddStackFeatures()
	server.AddLocalStackFeatures()
	server.AddSettingsFeatures()
	server.AddUserFeatures()
	server.AddTeamFeatures()
	server.AddAccessGroupFeatures()
	server.AddDockerProxyFeatures()
	server.AddKubernetesProxyFeatures()
}
