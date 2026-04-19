package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/toolgen"
	"gopkg.in/yaml.v3"
)

const generatedMarker = `AUTO-GENERATED — do not edit by hand.
Run: make regen-gateway-tools (or: go run ./cmd/regen-gateway-tools)
Source of truth: internal/tooldef/tools.yaml
The CI gateway-sync test will fail if this block drifts from source.`

type gatewayToolsFile struct {
	Tools []gatewayTool `yaml:"tools"`
}

type gatewayTool struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Arguments   []gatewayArgument   `yaml:"arguments,omitempty"`
	Annotations toolgen.Annotations `yaml:"annotations"`
}

type gatewayArgument struct {
	Name     string         `yaml:"name"`
	Type     string         `yaml:"type"`
	Desc     string         `yaml:"desc"`
	Optional bool           `yaml:"optional,omitempty"`
	Enum     []string       `yaml:"enum,omitempty"`
	Items    map[string]any `yaml:"items,omitempty"`
}

func main() {
	inputPath := flag.String("input", "internal/tooldef/tools.yaml", "Path to the source tools YAML file")
	gatewayPath := flag.String("gateway", "docker/portainer-mcp-gateway.yaml", "Path to the Docker MCP Toolkit gateway YAML file")
	outputPath := flag.String("output", "docker/generated/tools.yaml", "Path to the generated gateway tools YAML file (scratch artifact, can be empty to skip)")
	checkMode := flag.Bool("check", false, "Exit non-zero if the gateway YAML would be modified, do not write")
	showDiff := flag.Bool("diff", false, "Print a line-level diff of what would change (implies --check)")
	flag.Parse()

	logger := log.New(os.Stderr, "regen-gateway-tools: ", 0)

	if *showDiff {
		*checkMode = true
	}

	config, err := loadToolsConfig(*inputPath)
	if err != nil {
		logger.Fatalf("failed to load source tools from %s: %v", *inputPath, err)
	}

	tools := buildGatewayTools(config.Tools)

	scratchYAML, err := yaml.Marshal(tools)
	if err != nil {
		logger.Fatalf("failed to render gateway tools YAML: %v", err)
	}

	originalGateway, err := os.ReadFile(*gatewayPath)
	if err != nil {
		logger.Fatalf("failed to read gateway YAML at %s: %v", *gatewayPath, err)
	}

	updatedGateway, err := updateGatewayDocument(originalGateway, tools)
	if err != nil {
		logger.Fatalf("failed to update gateway YAML: %v", err)
	}

	if *checkMode {
		if bytes.Equal(originalGateway, updatedGateway) {
			logger.Printf("gateway tools block is in sync with %s", *inputPath)
			return
		}

		logger.Printf("DRIFT: %s is out of sync with %s", *gatewayPath, *inputPath)
		if *showDiff {
			fmt.Fprintln(os.Stderr, renderDiff(string(originalGateway), string(updatedGateway)))
		}
		logger.Print("run: make regen-gateway-tools")
		os.Exit(1)
	}

	if *outputPath != "" {
		if err := writeFile(*outputPath, scratchYAML); err != nil {
			logger.Fatalf("failed to write generated tools to %s: %v", *outputPath, err)
		}
	}

	if !bytes.Equal(originalGateway, updatedGateway) {
		if err := os.WriteFile(*gatewayPath, updatedGateway, 0644); err != nil {
			logger.Fatalf("failed to write updated gateway YAML to %s: %v", *gatewayPath, err)
		}
		logger.Printf("regenerated gateway tools metadata in %s", *gatewayPath)
		return
	}

	logger.Printf("gateway tools block already up to date in %s", *gatewayPath)
}

func loadToolsConfig(path string) (*toolgen.ToolsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config toolgen.ToolsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func buildGatewayTools(definitions []toolgen.ToolDefinition) gatewayToolsFile {
	tools := make([]gatewayTool, 0, len(definitions))
	for _, definition := range definitions {
		arguments := make([]gatewayArgument, 0, len(definition.Parameters))
		for _, parameter := range definition.Parameters {
			arguments = append(arguments, gatewayArgument{
				Name:     parameter.Name,
				Type:     parameter.Type,
				Desc:     parameter.Description,
				Optional: !parameter.Required,
				Enum:     parameter.Enum,
				Items:    parameter.Items,
			})
		}

		tool := gatewayTool{
			Name:        definition.Name,
			Description: definition.Description,
			Annotations: definition.Annotations,
		}
		if len(arguments) > 0 {
			tool.Arguments = arguments
		}

		tools = append(tools, tool)
	}

	return gatewayToolsFile{Tools: tools}
}

func updateGatewayDocument(existing []byte, tools gatewayToolsFile) ([]byte, error) {
	useCRLF := bytes.Contains(existing, []byte("\r\n"))
	normalized := existing
	if useCRLF {
		normalized = bytes.ReplaceAll(existing, []byte("\r\n"), []byte("\n"))
	}

	var document yaml.Node
	if err := yaml.Unmarshal(normalized, &document); err != nil {
		return nil, fmt.Errorf("parse gateway YAML: %w", err)
	}

	root, err := documentRoot(&document)
	if err != nil {
		return nil, err
	}

	toolsKey, toolsValue, err := findMappingEntry(root, "tools")
	if err != nil {
		return nil, err
	}

	var newToolsValue yaml.Node
	if err := newToolsValue.Encode(tools.Tools); err != nil {
		return nil, fmt.Errorf("encode tools list: %w", err)
	}

	*toolsValue = newToolsValue
	toolsKey.HeadComment = formatHeadComment(generatedMarker)

	rendered, err := marshalDocument(&document)
	if err != nil {
		return nil, err
	}

	if err := verifyToolsRoundTrip(rendered, tools); err != nil {
		return nil, fmt.Errorf("post-write verification failed: %w", err)
	}

	if useCRLF {
		rendered = bytes.ReplaceAll(rendered, []byte("\n"), []byte("\r\n"))
	}

	return rendered, nil
}

func documentRoot(node *yaml.Node) (*yaml.Node, error) {
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil, fmt.Errorf("gateway YAML document is empty")
		}
		return node.Content[0], nil
	}
	return node, nil
}

func findMappingEntry(mapping *yaml.Node, key string) (*yaml.Node, *yaml.Node, error) {
	if mapping.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("gateway YAML root is not a mapping (kind=%d)", mapping.Kind)
	}

	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index], mapping.Content[index+1], nil
		}
	}

	return nil, nil, fmt.Errorf("gateway YAML is missing top-level %q key", key)
}

func formatHeadComment(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for index, line := range lines {
		if line == "" {
			lines[index] = "#"
			continue
		}
		lines[index] = "# " + line
	}
	return strings.Join(lines, "\n")
}

func marshalDocument(document *yaml.Node) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		_ = encoder.Close()
		return nil, fmt.Errorf("encode gateway YAML: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}
	return buffer.Bytes(), nil
}

func verifyToolsRoundTrip(rendered []byte, expected gatewayToolsFile) error {
	var actual gatewayToolsFile
	if err := yaml.Unmarshal(rendered, &actual); err != nil {
		return fmt.Errorf("rendered output failed to parse: %w", err)
	}

	expectedYAML, err := yaml.Marshal(expected.Tools)
	if err != nil {
		return err
	}
	actualYAML, err := yaml.Marshal(actual.Tools)
	if err != nil {
		return err
	}

	if !bytes.Equal(expectedYAML, actualYAML) {
		return fmt.Errorf("rendered tools block does not round-trip to source representation")
	}

	return nil
}

func writeFile(path string, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

func renderDiff(original, updated string) string {
	originalLines := strings.Split(original, "\n")
	updatedLines := strings.Split(updated, "\n")

	var builder strings.Builder
	const maxDifferences = 40
	differencesShown := 0

	maxLines := len(originalLines)
	if len(updatedLines) > maxLines {
		maxLines = len(updatedLines)
	}

	for index := 0; index < maxLines; index++ {
		var originalLine, updatedLine string
		if index < len(originalLines) {
			originalLine = originalLines[index]
		}
		if index < len(updatedLines) {
			updatedLine = updatedLines[index]
		}

		if originalLine == updatedLine {
			continue
		}

		if differencesShown == 0 {
			builder.WriteString("--- current\n")
			builder.WriteString("+++ regenerated\n")
		}

		fmt.Fprintf(&builder, "@@ line %d @@\n- %s\n+ %s\n", index+1, originalLine, updatedLine)
		differencesShown++
		if differencesShown >= maxDifferences {
			fmt.Fprintf(&builder, "... %d more differences truncated ...\n", countRemainingDifferences(originalLines, updatedLines, index+1))
			break
		}
	}

	if differencesShown == 0 {
		return "(no line-level differences detected)"
	}

	return builder.String()
}

func countRemainingDifferences(originalLines, updatedLines []string, startIndex int) int {
	maxLines := len(originalLines)
	if len(updatedLines) > maxLines {
		maxLines = len(updatedLines)
	}

	remaining := 0
	for index := startIndex; index < maxLines; index++ {
		var originalLine, updatedLine string
		if index < len(originalLines) {
			originalLine = originalLines[index]
		}
		if index < len(updatedLines) {
			updatedLine = updatedLines[index]
		}
		if originalLine != updatedLine {
			remaining++
		}
	}
	return remaining
}
