package tooldef

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/toolgen"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type gatewayConfig struct {
	Tools []gatewayTool `yaml:"tools"`
}

type gatewayTool struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Arguments   []gatewayArgument   `yaml:"arguments"`
	Annotations toolgen.Annotations `yaml:"annotations"`
}

type gatewayArgument struct {
	Name     string         `yaml:"name"`
	Type     string         `yaml:"type"`
	Desc     string         `yaml:"desc"`
	Optional bool           `yaml:"optional"`
	Enum     []string       `yaml:"enum,omitempty"`
	Items    map[string]any `yaml:"items,omitempty"`
}

func TestGatewayMetadataMatchesEmbeddedTools(t *testing.T) {
	expectedTools := loadEmbeddedToolDefinitions(t)
	actualTools := loadGatewayToolDefinitions(t)

	require.Equal(t, expectedTools, actualTools)
}

func TestConvertGatewayPreservesItemsMapWithNestedValues(t *testing.T) {
	gateway := gatewayConfig{
		Tools: []gatewayTool{
			{
				Name:        "demo",
				Description: "demo tool",
				Arguments: []gatewayArgument{
					{
						Name:     "ids",
						Type:     "array",
						Desc:     "list of ids",
						Optional: false,
						Items:    map[string]any{"type": "integer"},
					},
					{
						Name:     "tags",
						Type:     "array",
						Desc:     "list of tag objects with nested enum",
						Optional: true,
						Items: map[string]any{
							"type": "string",
							"enum": []any{"prod", "dev"},
						},
					},
				},
			},
		},
	}

	converted := convertGatewayToToolDefinitions(gateway)
	require.Len(t, converted, 1)
	require.Len(t, converted[0].Parameters, 2)

	require.Equal(t, map[string]any{"type": "integer"}, converted[0].Parameters[0].Items)
	require.True(t, converted[0].Parameters[0].Required, "Optional=false in gateway must map to Required=true")

	require.Equal(t, "string", converted[0].Parameters[1].Items["type"])
	require.Equal(t, []any{"prod", "dev"}, converted[0].Parameters[1].Items["enum"])
	require.False(t, converted[0].Parameters[1].Required)
}

func TestGatewayConfigRoundTripsThroughYAMLWithItemsMap(t *testing.T) {
	original := gatewayConfig{
		Tools: []gatewayTool{
			{
				Name:        "withItems",
				Description: "exercises items map serialization",
				Arguments: []gatewayArgument{
					{
						Name:  "values",
						Type:  "array",
						Desc:  "values",
						Items: map[string]any{"type": "string"},
					},
				},
				Annotations: toolgen.Annotations{
					Title:        "Items Test",
					ReadOnlyHint: true,
				},
			},
		},
	}

	rendered, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded gatewayConfig
	require.NoError(t, yaml.Unmarshal(rendered, &decoded))

	require.Equal(t, original.Tools[0].Arguments[0].Items, decoded.Tools[0].Arguments[0].Items)
}

func loadEmbeddedToolDefinitions(t *testing.T) []toolgen.ToolDefinition {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "tooldef", "tools.yaml"))
	require.NoError(t, err)

	var config toolgen.ToolsConfig
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err)

	return normalizeToolDefinitions(config.Tools)
}

func loadGatewayToolDefinitions(t *testing.T) []toolgen.ToolDefinition {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "docker", "portainer-mcp-gateway.yaml"))
	require.NoError(t, err)

	var config gatewayConfig
	err = yaml.Unmarshal(data, &config)
	require.NoError(t, err)

	return normalizeToolDefinitions(convertGatewayToToolDefinitions(config))
}

func convertGatewayToToolDefinitions(config gatewayConfig) []toolgen.ToolDefinition {
	tools := make([]toolgen.ToolDefinition, 0, len(config.Tools))
	for _, tool := range config.Tools {
		parameters := make([]toolgen.ParameterDefinition, 0, len(tool.Arguments))
		for _, argument := range tool.Arguments {
			parameters = append(parameters, toolgen.ParameterDefinition{
				Name:        argument.Name,
				Type:        argument.Type,
				Required:    !argument.Optional,
				Enum:        argument.Enum,
				Description: argument.Desc,
				Items:       argument.Items,
			})
		}

		tools = append(tools, toolgen.ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  parameters,
			Annotations: tool.Annotations,
		})
	}

	return tools
}

func normalizeToolDefinitions(tools []toolgen.ToolDefinition) []toolgen.ToolDefinition {
	normalized := make([]toolgen.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		normalized = append(normalized, toolgen.ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  normalizeParameters(tool.Parameters),
			Annotations: tool.Annotations,
		})
	}

	slices.SortFunc(normalized, func(left, right toolgen.ToolDefinition) int {
		if left.Name < right.Name {
			return -1
		}
		if left.Name > right.Name {
			return 1
		}
		return 0
	})

	return normalized
}

func normalizeParameters(parameters []toolgen.ParameterDefinition) []toolgen.ParameterDefinition {
	if len(parameters) == 0 {
		return nil
	}

	normalized := make([]toolgen.ParameterDefinition, 0, len(parameters))
	for _, parameter := range parameters {
		items := parameter.Items
		if len(items) == 0 {
			items = nil
		}

		enum := parameter.Enum
		if len(enum) == 0 {
			enum = nil
		}

		normalized = append(normalized, toolgen.ParameterDefinition{
			Name:        parameter.Name,
			Type:        parameter.Type,
			Required:    parameter.Required,
			Enum:        enum,
			Description: parameter.Description,
			Items:       items,
		})
	}

	return normalized
}
