package mcp

import (
	"fmt"
	"log"
	"net/http"

	"github.com/Malaccamaxgit/portainer-mcp-safe/internal/safety"
	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/portainer/client"
	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/portainer/models"
	"github.com/Malaccamaxgit/portainer-mcp-safe/pkg/toolgen"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// MinimumToolsVersion is the minimum supported version of the tools.yaml file
	MinimumToolsVersion = "1.0"
	// SupportedPortainerVersion is the version of Portainer that is supported by this tool
	SupportedPortainerVersion = "2.31.2"
	// DefaultServerVersion is the fallback value advertised in the MCP
	// initialize handshake's serverInfo.version field when the binary's
	// build-time version has not been injected (e.g. plain `go test` runs).
	// The CLI in cmd/portainer-mcp assigns Version from main.Version so the
	// ldflag-supplied build version flows through to MCP clients.
	DefaultServerVersion = "dev"
)

// Version is the value reported as serverInfo.version in the MCP
// initialize handshake. The CLI overrides this with the binary's
// build-time version before constructing the server, keeping a single
// source of truth for what gets advertised to MCP clients.
var Version = DefaultServerVersion

// PortainerClient defines the interface for the wrapper client used by the MCP server
type PortainerClient interface {
	// Tag methods
	GetEnvironmentTags() ([]models.EnvironmentTag, error)
	CreateEnvironmentTag(name string) (int, error)

	// Environment methods
	GetEnvironments() ([]models.Environment, error)
	UpdateEnvironmentTags(id int, tagIds []int) error
	UpdateEnvironmentUserAccesses(id int, userAccesses map[int]string) error
	UpdateEnvironmentTeamAccesses(id int, teamAccesses map[int]string) error

	// Environment Group methods
	GetEnvironmentGroups() ([]models.Group, error)
	CreateEnvironmentGroup(name string, environmentIds []int) (int, error)
	UpdateEnvironmentGroupName(id int, name string) error
	UpdateEnvironmentGroupEnvironments(id int, environmentIds []int) error
	UpdateEnvironmentGroupTags(id int, tagIds []int) error

	// Access Group methods
	GetAccessGroups() ([]models.AccessGroup, error)
	CreateAccessGroup(name string, environmentIds []int) (int, error)
	UpdateAccessGroupName(id int, name string) error
	UpdateAccessGroupUserAccesses(id int, userAccesses map[int]string) error
	UpdateAccessGroupTeamAccesses(id int, teamAccesses map[int]string) error
	AddEnvironmentToAccessGroup(id int, environmentId int) error
	RemoveEnvironmentFromAccessGroup(id int, environmentId int) error

	// Stack methods (Edge Stacks)
	GetStacks() ([]models.Stack, error)
	GetStackFile(id int) (string, error)
	CreateStack(name string, file string, environmentGroupIds []int) (int, error)
	UpdateStack(id int, file string, environmentGroupIds []int) error

	// Local Stack methods (regular Docker Compose stacks)
	GetLocalStacks() ([]models.LocalStack, error)
	GetLocalStackFile(id int) (string, error)
	CreateLocalStack(endpointId int, name, file string, env []models.LocalStackEnvVar) (int, error)
	UpdateLocalStack(id, endpointId int, file string, env []models.LocalStackEnvVar, prune, pullImage bool) error
	StartLocalStack(id, endpointId int) error
	StopLocalStack(id, endpointId int) error
	DeleteLocalStack(id, endpointId int) error

	// Team methods
	CreateTeam(name string) (int, error)
	GetTeams() ([]models.Team, error)
	UpdateTeamName(id int, name string) error
	UpdateTeamMembers(id int, userIds []int) error

	// User methods
	GetUsers() ([]models.User, error)
	UpdateUserRole(id int, role string) error

	// Settings methods
	GetSettings() (models.PortainerSettings, error)

	// Version methods
	GetVersion() (string, error)

	// Docker Proxy methods
	ProxyDockerRequest(opts models.DockerProxyRequestOptions) (*http.Response, error)

	// Kubernetes Proxy methods
	ProxyKubernetesRequest(opts models.KubernetesProxyRequestOptions) (*http.Response, error)
}

// PortainerMCPServer is the main server that handles MCP protocol communication
// with AI assistants and translates them into Portainer API calls.
type PortainerMCPServer struct {
	srv             *server.MCPServer
	cli             PortainerClient
	tools           map[string]mcp.Tool
	toolDefinitions map[string]toolgen.ToolDefinition
	registeredTools map[string]struct{}
	readOnly        bool
	businessEdition bool
	policy          *safety.Policy
}

// ServerOption is a function that configures the server
type ServerOption func(*serverOptions)

// serverOptions contains all configurable options for the server
type serverOptions struct {
	client              PortainerClient
	readOnly            bool
	businessEdition     bool
	disableVersionCheck bool
	safetyConfig        safety.Config
}

// WithClient sets a custom client for the server.
// This is primarily used for testing to inject mock clients.
func WithClient(client PortainerClient) ServerOption {
	return func(opts *serverOptions) {
		opts.client = client
	}
}

// WithReadOnly sets the server to read-only mode.
// This will prevent the server from registering write tools.
func WithReadOnly(readOnly bool) ServerOption {
	return func(opts *serverOptions) {
		opts.readOnly = readOnly
	}
}

func WithBusinessEdition(enabled bool) ServerOption {
	return func(opts *serverOptions) {
		opts.businessEdition = enabled
	}
}

// WithDisableVersionCheck disables the Portainer server version check.
// This allows connecting to unsupported Portainer versions.
func WithDisableVersionCheck(disable bool) ServerOption {
	return func(opts *serverOptions) {
		opts.disableVersionCheck = disable
	}
}

func WithSafeMode(enabled bool) ServerOption {
	return func(opts *serverOptions) {
		opts.safetyConfig.SafeMode = enabled
	}
}

func WithAllowUnredactedStackContent(enabled bool) ServerOption {
	return func(opts *serverOptions) {
		opts.safetyConfig.AllowUnredactedStackContent = enabled
	}
}

func WithAllowSensitiveProxyPaths(enabled bool) ServerOption {
	return func(opts *serverOptions) {
		opts.safetyConfig.AllowSensitiveProxyPaths = enabled
	}
}

func WithProxyAllowlist(entries []string) ServerOption {
	return func(opts *serverOptions) {
		opts.safetyConfig.ProxyAllowlist = entries
	}
}

func WithExtraRedactionPatterns(patterns []string) ServerOption {
	return func(opts *serverOptions) {
		opts.safetyConfig.ExtraRedactionPatterns = patterns
	}
}

// NewPortainerMCPServer creates a new Portainer MCP server.
//
// This server provides an implementation of the MCP protocol for Portainer,
// allowing AI assistants to interact with Portainer through a structured API.
//
// Parameters:
//   - serverURL: The base URL of the Portainer server (e.g., "https://portainer.example.com")
//   - token: The API token for authenticating with the Portainer server
//   - toolsPath: Path to the tools.yaml file that defines the available MCP tools
//   - options: Optional functional options for customizing server behavior (e.g., WithClient)
//
// Returns:
//   - A configured PortainerMCPServer instance ready to be started
//   - An error if initialization fails
//
// Possible errors:
//   - Failed to load tools from the specified path
//   - Failed to communicate with the Portainer server
//   - Incompatible Portainer server version
func NewPortainerMCPServer(serverURL, token, toolsPath string, options ...ServerOption) (*PortainerMCPServer, error) {
	opts := &serverOptions{
		safetyConfig: safety.Config{
			SafeMode: true,
		},
	}

	for _, option := range options {
		option(opts)
	}

	tools, toolDefinitions, err := toolgen.LoadToolsFromYAML(toolsPath, MinimumToolsVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to load tools: %w", err)
	}

	var portainerClient PortainerClient
	if opts.client != nil {
		portainerClient = opts.client
	} else {
		portainerClient = client.NewPortainerClient(serverURL, token, client.WithSkipTLSVerify(true))
	}

	if !opts.disableVersionCheck {
		version, err := portainerClient.GetVersion()
		if err != nil {
			return nil, fmt.Errorf("failed to get Portainer server version: %w", err)
		}

		if version != SupportedPortainerVersion {
			return nil, fmt.Errorf("unsupported Portainer server version: %s, only version %s is supported", version, SupportedPortainerVersion)
		}
	}

	policy, err := safety.NewPolicy(opts.safetyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize safety policy: %w", err)
	}

	return &PortainerMCPServer{
		srv: server.NewMCPServer(
			"Portainer MCP Safe Server",
			Version,
			server.WithToolCapabilities(true),
			server.WithLogging(),
		),
		cli:             portainerClient,
		tools:           tools,
		toolDefinitions: toolDefinitions,
		registeredTools: make(map[string]struct{}),
		readOnly:        opts.readOnly,
		businessEdition: opts.businessEdition,
		policy:          policy,
	}, nil
}

// Start begins listening for MCP protocol messages on standard input/output.
// This is a blocking call that will run until the connection is closed.
func (s *PortainerMCPServer) Start() error {
	return server.ServeStdio(s.srv)
}

// addToolIfExists adds a tool to the server if it exists in the tools map
func (s *PortainerMCPServer) addToolIfExists(toolName string, handler server.ToolHandlerFunc) {
	tool, exists := s.tools[toolName]
	if !exists {
		log.Printf("Tool %s not found, will not be registered for MCP usage", toolName)
		return
	}

	toolDefinition, exists := s.toolDefinitions[toolName]
	if exists && toolDefinition.RequiresBusinessEdition && !s.businessEdition {
		log.Printf("Tool %s requires Portainer Business Edition and will not be registered", toolName)
		return
	}

	s.srv.AddTool(tool, handler)
	if s.registeredTools == nil {
		s.registeredTools = make(map[string]struct{})
	}
	s.registeredTools[toolName] = struct{}{}
}

func (s *PortainerMCPServer) safetyPolicy() *safety.Policy {
	if s.policy != nil {
		return s.policy
	}

	policy, err := safety.NewPolicy(safety.Config{})
	if err != nil {
		return nil
	}

	s.policy = policy
	return s.policy
}
