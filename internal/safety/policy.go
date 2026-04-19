package safety

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/portainer/models"
	"gopkg.in/yaml.v3"
)

const redactedValue = "<redacted>"

var defaultRedactionPatterns = []string{
	`(?i)(password|secret|token|key|credential|passwd|apikey|auth)`,
}

var defaultDockerAllowlist = []ProxyRule{
	{Method: "GET", PathPrefix: "/version"},
	{Method: "GET", PathPrefix: "/info"},
	{Method: "GET", PathPrefix: "/containers/json"},
	{Method: "GET", PathPrefix: "/images/json"},
	{Method: "GET", PathPrefix: "/networks"},
	{Method: "GET", PathPrefix: "/volumes"},
}

type Config struct {
	SafeMode                    bool
	AllowUnredactedStackContent bool
	AllowSensitiveProxyPaths    bool
	ProxyAllowlist              []string
	ExtraRedactionPatterns      []string
}

type ProxyRule struct {
	Method     string
	PathPrefix string
}

type Note struct {
	SafeMode     bool     `json:"safeMode"`
	Redacted     bool     `json:"redacted,omitempty"`
	Denied       bool     `json:"denied,omitempty"`
	Notes        []string `json:"notes,omitempty"`
	RedactedKeys []string `json:"redactedKeys,omitempty"`
}

type Decision struct {
	Allowed bool
	Message string
	Note    *Note
}

type Policy struct {
	config           Config
	patterns         []*regexp.Regexp
	customProxyRules []ProxyRule
}

type collector struct {
	keys []string
}

func NewPolicy(config Config) (*Policy, error) {
	patterns := make([]*regexp.Regexp, 0, len(defaultRedactionPatterns)+len(config.ExtraRedactionPatterns))
	for _, pattern := range append(slices.Clone(defaultRedactionPatterns), config.ExtraRedactionPatterns...) {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("compile redaction pattern %q: %w", pattern, err)
		}
		patterns = append(patterns, compiled)
	}

	customProxyRules := make([]ProxyRule, 0, len(config.ProxyAllowlist))
	for _, entry := range config.ProxyAllowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		rule, err := parseProxyRule(entry)
		if err != nil {
			return nil, err
		}
		customProxyRules = append(customProxyRules, rule)
	}

	return &Policy{
		config:           config,
		patterns:         patterns,
		customProxyRules: customProxyRules,
	}, nil
}

func (p *Policy) SanitizeLocalStacks(stacks []models.LocalStack) ([]models.LocalStack, *Note) {
	if !p.config.SafeMode || p.config.AllowUnredactedStackContent {
		return stacks, nil
	}

	redactedStacks := make([]models.LocalStack, len(stacks))
	copy(redactedStacks, stacks)

	state := collector{}
	redactedCount := 0

	for index, stack := range redactedStacks {
		if len(stack.Env) == 0 {
			continue
		}

		envCopy := make([]models.LocalStackEnvVar, len(stack.Env))
		copy(envCopy, stack.Env)

		for envIndex, envVar := range envCopy {
			if p.shouldRedactKey(envVar.Name) {
				envCopy[envIndex].Value = redactedValue
				state.add(envVar.Name)
				redactedCount++
			}
		}

		redactedStacks[index].Env = envCopy
	}

	if redactedCount == 0 {
		return redactedStacks, nil
	}

	return redactedStacks, &Note{
		SafeMode:     true,
		Redacted:     true,
		RedactedKeys: state.sortedKeys(),
		Notes: []string{
			fmt.Sprintf("redacted %d local stack environment value(s) that matched the configured secret patterns", redactedCount),
		},
	}
}

func (p *Policy) SanitizeComposeContent(content string) (string, *Note, error) {
	if !p.config.SafeMode || p.config.AllowUnredactedStackContent {
		return content, nil, nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return "", nil, fmt.Errorf("parse compose content in safe mode: %w", err)
	}

	state := collector{}
	redactions := p.redactYAMLNode(&root, &state)
	if redactions == 0 {
		return content, nil, nil
	}

	output, err := yaml.Marshal(&root)
	if err != nil {
		return "", nil, fmt.Errorf("marshal redacted compose content: %w", err)
	}

	return string(output), &Note{
		SafeMode:     true,
		Redacted:     true,
		RedactedKeys: state.sortedKeys(),
		Notes: []string{
			fmt.Sprintf("redacted %d compose value(s) that matched the configured stack safety policy", redactions),
		},
	}, nil
}

func (p *Policy) CheckDockerProxy(method, path string) *Decision {
	if !p.config.SafeMode || p.config.AllowSensitiveProxyPaths {
		return nil
	}

	if p.isAllowedProxyPath(method, path, defaultDockerAllowlist) {
		return nil
	}

	return &Decision{
		Allowed: false,
		Message: fmt.Sprintf("docker proxy path denied by safe_mode: %s %s is not in the allowlist", strings.ToUpper(method), path),
		Note: &Note{
			SafeMode: true,
			Denied:   true,
			Notes: []string{
				fmt.Sprintf("blocked docker proxy request %s %s because it is not in the safe_mode allowlist", strings.ToUpper(method), path),
			},
		},
	}
}

func (p *Policy) CheckKubernetesProxy(method, path string) *Decision {
	if !p.config.SafeMode || p.config.AllowSensitiveProxyPaths {
		return nil
	}

	lowerPath := strings.ToLower(path)
	if strings.Contains(lowerPath, "/secrets") || strings.HasSuffix(lowerPath, "/secret") {
		return &Decision{
			Allowed: false,
			Message: fmt.Sprintf("kubernetes proxy path denied by safe_mode: %s %s targets secrets", strings.ToUpper(method), path),
			Note: &Note{
				SafeMode: true,
				Denied:   true,
				Notes: []string{
					fmt.Sprintf("blocked kubernetes proxy request %s %s because secret resources are denied in safe_mode", strings.ToUpper(method), path),
				},
			},
		}
	}

	return nil
}

func (p *Policy) SanitizeKubernetesJSON(body []byte) ([]byte, *Note, error) {
	if !p.config.SafeMode {
		return body, nil, nil
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return body, nil, nil
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil, nil
	}

	state := collector{}
	redactedPayload := p.redactJSONValue(payload, &state)
	if len(state.keys) == 0 {
		return body, nil, nil
	}

	output, err := json.Marshal(redactedPayload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal redacted kubernetes response JSON: %w", err)
	}

	return output, &Note{
		SafeMode:     true,
		Redacted:     true,
		RedactedKeys: state.sortedKeys(),
		Notes: []string{
			fmt.Sprintf("redacted %d kubernetes field(s) that matched the configured secret patterns", len(state.sortedKeys())),
		},
	}, nil
}

func (p *Policy) redactYAMLNode(node *yaml.Node, state *collector) int {
	if node == nil {
		return 0
	}

	switch node.Kind {
	case yaml.DocumentNode:
		redactions := 0
		for _, child := range node.Content {
			redactions += p.redactYAMLNode(child, state)
		}
		return redactions
	case yaml.MappingNode:
		redactions := 0
		for index := 0; index < len(node.Content); index += 2 {
			keyNode := node.Content[index]
			valueNode := node.Content[index+1]

			switch keyNode.Value {
			case "environment":
				redactions += p.redactEnvironmentNode(valueNode, state)
			case "secrets":
				redactions += redactSecretsNode(valueNode, state)
			default:
				redactions += p.redactYAMLNode(valueNode, state)
			}
		}
		return redactions
	case yaml.SequenceNode:
		redactions := 0
		for _, child := range node.Content {
			redactions += p.redactYAMLNode(child, state)
		}
		return redactions
	default:
		return 0
	}
}

func (p *Policy) redactEnvironmentNode(node *yaml.Node, state *collector) int {
	if node == nil {
		return 0
	}

	switch node.Kind {
	case yaml.MappingNode:
		redactions := 0
		for index := 0; index < len(node.Content); index += 2 {
			keyNode := node.Content[index]
			valueNode := node.Content[index+1]
			if p.shouldRedactKey(keyNode.Value) {
				valueNode.Kind = yaml.ScalarNode
				valueNode.Tag = "!!str"
				valueNode.Value = redactedValue
				valueNode.Content = nil
				state.add(keyNode.Value)
				redactions++
			}
		}
		return redactions
	case yaml.SequenceNode:
		redactions := 0
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				continue
			}
			key, _, found := strings.Cut(item.Value, "=")
			if !found || !p.shouldRedactKey(key) {
				continue
			}
			item.Value = fmt.Sprintf("%s=%s", key, redactedValue)
			state.add(key)
			redactions++
		}
		return redactions
	default:
		return 0
	}
}

func redactSecretsNode(node *yaml.Node, state *collector) int {
	if node == nil {
		return 0
	}

	state.add("secrets")

	switch node.Kind {
	case yaml.MappingNode:
		redactions := len(node.Content) / 2
		for index := 1; index < len(node.Content); index += 2 {
			node.Content[index].Kind = yaml.ScalarNode
			node.Content[index].Tag = "!!str"
			node.Content[index].Value = redactedValue
			node.Content[index].Content = nil
		}
		return redactions
	case yaml.SequenceNode:
		redactions := len(node.Content)
		for _, item := range node.Content {
			item.Kind = yaml.ScalarNode
			item.Tag = "!!str"
			item.Value = redactedValue
			item.Content = nil
		}
		return redactions
	default:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!str"
		node.Value = redactedValue
		node.Content = nil
		return 1
	}
}

func (p *Policy) redactJSONValue(value any, state *collector) any {
	switch typed := value.(type) {
	case map[string]any:
		kind, _ := typed["kind"].(string)
		for key, child := range typed {
			if strings.EqualFold(kind, "Secret") && (key == "data" || key == "stringData") {
				typed[key] = redactedValue
				state.add(fmt.Sprintf("%s.%s", kind, key))
				continue
			}
			if p.shouldRedactKey(key) {
				typed[key] = redactedValue
				state.add(key)
				continue
			}
			typed[key] = p.redactJSONValue(child, state)
		}
		return typed
	case []any:
		for index, child := range typed {
			typed[index] = p.redactJSONValue(child, state)
		}
		return typed
	default:
		return value
	}
}

func (p *Policy) shouldRedactKey(key string) bool {
	for _, pattern := range p.patterns {
		if pattern.MatchString(key) {
			return true
		}
	}
	return false
}

func (p *Policy) isAllowedProxyPath(method, path string, defaultRules []ProxyRule) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	for _, rule := range append(slices.Clone(defaultRules), p.customProxyRules...) {
		if rule.Method == method && strings.HasPrefix(path, rule.PathPrefix) {
			return true
		}
	}
	return false
}

func parseProxyRule(entry string) (ProxyRule, error) {
	method, pathPrefix, found := strings.Cut(entry, ":")
	if !found {
		return ProxyRule{}, fmt.Errorf("invalid proxy allowlist entry %q: expected METHOD:/path-prefix", entry)
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	pathPrefix = strings.TrimSpace(pathPrefix)

	if method == "" || pathPrefix == "" || !strings.HasPrefix(pathPrefix, "/") {
		return ProxyRule{}, fmt.Errorf("invalid proxy allowlist entry %q: expected METHOD:/path-prefix", entry)
	}

	return ProxyRule{
		Method:     method,
		PathPrefix: pathPrefix,
	}, nil
}

func (c *collector) add(key string) {
	c.keys = append(c.keys, key)
}

func (c *collector) sortedKeys() []string {
	if len(c.keys) == 0 {
		return nil
	}

	deduped := make(map[string]struct{}, len(c.keys))
	for _, key := range c.keys {
		deduped[key] = struct{}{}
	}

	keys := make([]string, 0, len(deduped))
	for key := range deduped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
