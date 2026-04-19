package main

import (
	"flag"
	"strings"

	"github.com/Malaccamaxgit/portainer-mcp-safe/internal/mcp"
	"github.com/Malaccamaxgit/portainer-mcp-safe/internal/tooldef"
	"github.com/rs/zerolog/log"
)

const defaultToolsPath = "tools.yaml"

var (
	Version   string
	BuildDate string
	Commit    string
)

func main() {
	log.Info().
		Str("version", Version).
		Str("build-date", BuildDate).
		Str("commit", Commit).
		Msg("Portainer MCP server")

	if Version != "" {
		mcp.Version = Version
	}

	serverFlag := flag.String("server", "", "The Portainer server URL")
	tokenFlag := flag.String("token", "", "The authentication token for the Portainer server")
	toolsFlag := flag.String("tools", "", "The path to the tools YAML file")
	readOnlyFlag := flag.Bool("read-only", false, "Run in read-only mode")
	businessEditionFlag := flag.Bool("business-edition", false, "Enable Portainer Business Edition tool registration")
	disableVersionCheckFlag := flag.Bool("disable-version-check", false, "Disable Portainer server version check")
	safeModeFlag := flag.Bool("safe-mode", true, "Enable safe-mode redaction and proxy guards")
	allowUnredactedStackContentFlag := flag.Bool("allow-unredacted-stack-content", false, "Allow unredacted stack environment values and compose content in safe mode")
	allowSensitiveProxyPathsFlag := flag.Bool("allow-sensitive-proxy-paths", false, "Allow sensitive Docker and Kubernetes proxy paths in safe mode")
	proxyAllowlistFlag := flag.String("proxy-allowlist", "", "Additional proxy allowlist entries formatted as METHOD:/path-prefix and separated by commas")
	extraRedactionPatternsFlag := flag.String("extra-redaction-patterns", "", "Additional redaction regex patterns separated by commas")

	flag.Parse()

	if *serverFlag == "" || *tokenFlag == "" {
		log.Fatal().Msg("Both -server and -token flags are required")
	}

	toolsPath := *toolsFlag
	if toolsPath == "" {
		toolsPath = defaultToolsPath
	}

	// We first check if the tools.yaml file exists
	// We'll create it from the embedded version if it doesn't exist
	exists, err := tooldef.CreateToolsFileIfNotExists(toolsPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create tools.yaml file")
	}

	if exists {
		log.Info().Msg("using existing tools.yaml file")
	} else {
		log.Info().Msg("created tools.yaml file")
	}

	log.Info().
		Str("portainer-host", *serverFlag).
		Str("tools-path", toolsPath).
		Bool("read-only", *readOnlyFlag).
		Bool("business-edition", *businessEditionFlag).
		Bool("disable-version-check", *disableVersionCheckFlag).
		Bool("safe-mode", *safeModeFlag).
		Bool("allow-unredacted-stack-content", *allowUnredactedStackContentFlag).
		Bool("allow-sensitive-proxy-paths", *allowSensitiveProxyPathsFlag).
		Strs("proxy-allowlist", splitCSV(*proxyAllowlistFlag)).
		Strs("extra-redaction-patterns", splitCSV(*extraRedactionPatternsFlag)).
		Msg("starting MCP server")

	server, err := mcp.NewPortainerMCPServer(
		*serverFlag,
		*tokenFlag,
		toolsPath,
		mcp.WithReadOnly(*readOnlyFlag),
		mcp.WithBusinessEdition(*businessEditionFlag),
		mcp.WithDisableVersionCheck(*disableVersionCheckFlag),
		mcp.WithSafeMode(*safeModeFlag),
		mcp.WithAllowUnredactedStackContent(*allowUnredactedStackContentFlag),
		mcp.WithAllowSensitiveProxyPaths(*allowSensitiveProxyPathsFlag),
		mcp.WithProxyAllowlist(splitCSV(*proxyAllowlistFlag)),
		mcp.WithExtraRedactionPatterns(splitCSV(*extraRedactionPatternsFlag)),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create server")
	}

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

	err = server.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start server")
	}
}

func splitCSV(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}

	parts := strings.Split(input, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}

	return values
}
