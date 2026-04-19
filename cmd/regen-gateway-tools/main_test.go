package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/toolgen"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const minimalGatewayYAML = `name: portainer-mcp-safe
title: Test
description: Test gateway
command:
  - /usr/local/bin/docker-entrypoint.sh
tools:
  - name: stale
    description: this should be replaced entirely
    annotations:
      title: Stale
      readOnlyHint: true
      destructiveHint: false
      idempotentHint: true
      openWorldHint: false
`

func sampleConfig() *toolgen.ToolsConfig {
	return &toolgen.ToolsConfig{
		Version: "1.0",
		Tools: []toolgen.ToolDefinition{
			{
				Name:        "listEnvironments",
				Description: "List environments",
				Parameters:  nil,
				Annotations: toolgen.Annotations{
					Title:           "List Environments",
					ReadOnlyHint:    true,
					DestructiveHint: false,
					IdempotentHint:  true,
					OpenWorldHint:   false,
				},
			},
			{
				Name:        "createEnvironment",
				Description: "Create an environment",
				Parameters: []toolgen.ParameterDefinition{
					{
						Name:        "name",
						Type:        "string",
						Required:    true,
						Description: "Name",
					},
					{
						Name:        "tagIDs",
						Type:        "array",
						Required:    false,
						Description: "Tag IDs to attach",
						Items:       map[string]any{"type": "integer"},
					},
					{
						Name:        "kind",
						Type:        "string",
						Required:    true,
						Description: "Kind",
						Enum:        []string{"docker", "kubernetes"},
					},
				},
				Annotations: toolgen.Annotations{
					Title:           "Create Environment",
					ReadOnlyHint:    false,
					DestructiveHint: true,
					IdempotentHint:  false,
					OpenWorldHint:   true,
				},
			},
		},
	}
}

func TestBuildGatewayToolsInvertsRequiredAndPreservesOptionalEmpty(t *testing.T) {
	tools := buildGatewayTools(sampleConfig().Tools)

	require.Len(t, tools.Tools, 2)

	listTool := tools.Tools[0]
	require.Equal(t, "listEnvironments", listTool.Name)
	require.Empty(t, listTool.Arguments, "tools without parameters must not emit an arguments block")

	createTool := tools.Tools[1]
	require.Len(t, createTool.Arguments, 3)

	nameArg := createTool.Arguments[0]
	require.Equal(t, "name", nameArg.Name)
	require.False(t, nameArg.Optional, "required source params must not be marked optional")

	tagArg := createTool.Arguments[1]
	require.Equal(t, "tagIDs", tagArg.Name)
	require.True(t, tagArg.Optional, "non-required source params must be marked optional")
	require.Equal(t, map[string]any{"type": "integer"}, tagArg.Items)

	kindArg := createTool.Arguments[2]
	require.True(t, kindArg.Name == "kind")
	require.Equal(t, []string{"docker", "kubernetes"}, kindArg.Enum)
}

func TestUpdateGatewayDocumentReplacesToolsAndAddsMarker(t *testing.T) {
	tools := buildGatewayTools(sampleConfig().Tools)

	updated, err := updateGatewayDocument([]byte(minimalGatewayYAML), tools)
	require.NoError(t, err)

	updatedString := string(updated)
	require.Contains(t, updatedString, "# AUTO-GENERATED")
	require.Contains(t, updatedString, "make regen-gateway-tools")
	require.NotContains(t, updatedString, "this should be replaced entirely",
		"stale tool block must be fully replaced")
	require.Contains(t, updatedString, "listEnvironments")
	require.Contains(t, updatedString, "createEnvironment")

	require.Contains(t, updatedString, "name: portainer-mcp-safe",
		"non-tools content must be preserved")
}

func TestUpdateGatewayDocumentIsIdempotent(t *testing.T) {
	tools := buildGatewayTools(sampleConfig().Tools)

	first, err := updateGatewayDocument([]byte(minimalGatewayYAML), tools)
	require.NoError(t, err)

	second, err := updateGatewayDocument(first, tools)
	require.NoError(t, err)

	require.Equal(t, string(first), string(second),
		"running regen on an already-synced document must not modify it")
}

func TestUpdateGatewayDocumentPreservesCRLFLineEndings(t *testing.T) {
	crlfInput := strings.ReplaceAll(minimalGatewayYAML, "\n", "\r\n")
	tools := buildGatewayTools(sampleConfig().Tools)

	updated, err := updateGatewayDocument([]byte(crlfInput), tools)
	require.NoError(t, err)

	require.Contains(t, string(updated), "\r\n",
		"CRLF input must produce CRLF output")
	require.NotContains(t,
		strings.ReplaceAll(string(updated), "\r\n", ""),
		"\n",
		"output must not mix LF and CRLF")
}

func TestUpdateGatewayDocumentRejectsMissingToolsKey(t *testing.T) {
	noToolsYAML := "name: foo\ndescription: bar\n"
	_, err := updateGatewayDocument([]byte(noToolsYAML), buildGatewayTools(sampleConfig().Tools))
	require.Error(t, err)
	require.Contains(t, err.Error(), "tools")
}

func TestUpdateGatewayDocumentRejectsNonMappingRoot(t *testing.T) {
	listYAML := "- one\n- two\n"
	_, err := updateGatewayDocument([]byte(listYAML), buildGatewayTools(sampleConfig().Tools))
	require.Error(t, err)
	require.Contains(t, err.Error(), "mapping")
}

func TestUpdatedDocumentRoundTripsThroughYAML(t *testing.T) {
	tools := buildGatewayTools(sampleConfig().Tools)
	updated, err := updateGatewayDocument([]byte(minimalGatewayYAML), tools)
	require.NoError(t, err)

	var parsed gatewayToolsFile
	require.NoError(t, yaml.Unmarshal(updated, &parsed))
	require.Equal(t, tools, parsed,
		"rendered YAML must round-trip back to the same gateway tool structure")
}

func TestRenderDiffReportsLineLevelChanges(t *testing.T) {
	original := "alpha\nbeta\ngamma\n"
	updated := "alpha\nBETA\ngamma\n"

	diff := renderDiff(original, updated)
	require.Contains(t, diff, "@@ line 2 @@")
	require.Contains(t, diff, "- beta")
	require.Contains(t, diff, "+ BETA")
}

func TestRenderDiffOnIdenticalReturnsSentinel(t *testing.T) {
	identical := "one\ntwo\n"
	require.Equal(t, "(no line-level differences detected)", renderDiff(identical, identical))
}

func TestFormatHeadCommentPrefixesAllLines(t *testing.T) {
	got := formatHeadComment("first line\n\nthird line")
	require.Equal(t, "# first line\n#\n# third line", got)
}

func TestVerifyToolsRoundTripCatchesMismatch(t *testing.T) {
	tools := buildGatewayTools(sampleConfig().Tools)
	good, err := updateGatewayDocument([]byte(minimalGatewayYAML), tools)
	require.NoError(t, err)
	require.NoError(t, verifyToolsRoundTrip(good, tools))

	corrupted := bytes.ReplaceAll(good, []byte("listEnvironments"), []byte("listEnvironmentsXX"))
	require.Error(t, verifyToolsRoundTrip(corrupted, tools))
}
