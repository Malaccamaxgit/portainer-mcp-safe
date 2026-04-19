# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Portainer MCP Development Guide

## Build, Test & Run Commands

**Prerequisites**: Go 1.24+, golangci-lint, Docker (for integration tests), npx (for inspector)

### Build Commands
- Build: `make build`
- Build for specific platform: `make PLATFORM=<platform> ARCH=<arch> build`
- Clean build artifacts: `make clean`

### Test Commands
- Unit tests: `make test` (excludes integration tests)
- Run single test: `go test -v ./path/to/package -run TestName`
- Test with coverage: `make test-coverage` (generates `coverage.out`)
- Integration tests: `make test-integration` (requires Docker; spins up Portainer containers)
- Run all tests: `make test-all` (unit + integration)

### Docker Builds
- Build test image: `docker build -f docker/Dockerfile --target test -t portainer-mcp-safe-test .`
- Run tests in container: `docker run --rm portainer-mcp-safe-test`
- Build runtime image: `docker build -f docker/Dockerfile -t portainer-mcp-safe:0.7.0-safe.1 .`

### Development Tools
- Lint: `make lint` (runs `golangci-lint run ./...`)
- Format code: `gofmt -s -w .`
- Run inspector: `make inspector`
- Regenerate gateway tools: `make regen-gateway-tools`

### CLI Flags
```
dist/portainer-mcp \
  -server <portainer-url> \   # Required: Portainer server URL
  -token <api-token> \        # Required: Portainer API token
  -tools <path>               # Optional: path to tools.yaml (default: tools.yaml)
  -read-only                  # Optional: restrict to read-only tools
  -business-edition           # Optional: enable Business Edition-only tools
  -disable-version-check      # Optional: skip Portainer version validation
  -safe-mode=true             # Enable safe-mode redaction and proxy guards (default: true)
  -allow-unredacted-stack-content=false  # Allow unredacted stack env values in safe mode
  -allow-sensitive-proxy-paths=false     # Allow sensitive Docker/K8s proxy paths in safe mode
  -proxy-allowlist ""         # Extra METHOD:/path-prefix entries, comma-separated
  -extra-redaction-patterns "" # Extra redaction regex patterns, comma-separated
```

## High-Level Architecture

### Application Flow
```
cmd/portainer-mcp/mcp.go (entry point)
    ↓
internal/mcp/server.go (MCPServer initialization)
    ↓
pkg/portainer/client/ (API client wrapper)
    ↓
internal/mcp/*.go (feature handlers)
    ↓
Portainer REST API / Docker API / Kubernetes API
```

### Package Responsibilities

| Package | Purpose |
|---------|---------|
| `cmd/portainer-mcp` | Main entry point, CLI flags, feature registration |
| `cmd/regen-gateway-tools` | Syncs tool definitions to Docker gateway YAML |
| `internal/mcp` | MCP server core + feature handlers (docker.go, kubernetes.go, local_stack.go, etc.) |
| `internal/safety` | Redaction patterns, Docker/K8s proxy allowlisting, secret blocking |
| `internal/tooldef` | Embedded tools.yaml loading |
| `internal/k8sutil` | Kubernetes metadata stripping |
| `pkg/portainer/client` | Portainer API wrapper (SDK + raw HTTP paths) |
| `pkg/portainer/models` | Local simplified models + conversions |
| `pkg/toolgen` | YAML parsing, tool schema generation |

### Two API Implementation Paths

The `PortainerClient` holds two internal clients because the SDK doesn't cover the full Portainer API:

#### Path 1: SDK Client (preferred)
- Uses `github.com/portainer/client-api-go/v2`
- Covers: environments, tags, teams, users, groups, access groups, edge stacks, settings, Docker/K8s proxy
- Pattern: `c.cli.SomeMethod()` → convert to local models → return
- Example files: `client/environment.go`, `client/tag.go`, `client/stack.go`
- **Unit tests**: mock `PortainerAPIClient` interface via `MockPortainerAPI`

#### Path 2: Raw HTTP Client (when SDK lacks methods)
- Uses `c.rawCli.apiRequest(method, path, body)`
- Used for: local (non-edge) Docker Compose stacks (`/api/stacks/*`)
- Authenticates with `X-API-Key` header
- Define request/response structs locally
- Example: `client/local_stack.go`
- **Unit tests**: use `httptest.NewServer`

### Safety Policy Architecture

Safe mode (enabled by default) provides:

1. **Stack Environment Redaction** (`policy.go:SanitizeLocalStacks`):
   - Redacts values matching patterns: password, secret, token, key, credential, etc.
   - Applies to `ListLocalStacks` and `GetLocalStackFile` output

2. **Compose Content Redaction** (`policy.go:SanitizeComposeContent`):
   - Parses YAML, redacts `environment` and `secrets` sections
   - Returns redacted YAML + note about what was redacted

3. **Docker Proxy Allowlisting** (`policy.go:CheckDockerProxy`):
   - Default allowlist: GET /version, /info, /containers/json, /images/json, /networks, /volumes
   - Blocks all other paths unless explicitly allowed

4. **Kubernetes Secret Blocking** (`policy.go:CheckKubernetesProxy`):
   - Blocks any path containing `/secrets` or ending with `/secret`
   - JSON response redaction via `SanitizeKubernetesJSON`

### MCP Server Feature Domains

The server registers these feature groups (see `cmd/portainer-mcp/mcp.go`):

| Feature | Files | Edition |
|---------|-------|---------|
| Environments | `mcp/environment.go` | CE |
| Tags | `mcp/tag.go` | CE |
| Teams | `mcp/team.go` | CE |
| Users | `mcp/user.go` | CE |
| Settings | `mcp/settings.go` | CE |
| Local Stacks | `mcp/local_stack.go` | CE |
| Docker Proxy | `mcp/docker.go` | CE |
| Kubernetes Proxy | `mcp/kubernetes.go` | CE |
| Edge Stacks | `mcp/stack.go` | EE |
| Environment Groups | `mcp/group.go` | EE |
| Access Groups | `mcp/access_group.go` | EE |

**Tool counts**: 39 total → 21 in CE mode → 10 in read-only + CE mode

## Code Style Guidelines

- Use standard Go naming conventions: PascalCase for exported, camelCase for private
- Follow table-driven test pattern with descriptive test cases
- Error handling: return errors with context via `fmt.Errorf("failed to X: %w", err)`
- Imports: group standard library, external packages, and internal packages
- Function comments: document exported functions with Parameters/Returns sections
- Use functional options pattern for configurable clients
- Lint with `make lint` before committing; fix all warnings

### Import Conventions
```go
import (
    "github.com/Malaccamaxgit/portainer-mcp-safe/pkg/portainer/models" // Local MCP models
    apimodels "github.com/portainer/client-api-go/v2/pkg/models"      // Raw SDK models
)
```

## Testing Conventions

### Unit Tests by API Path
- **SDK path**: mock `PortainerAPIClient` interface, assert mock expectations
- **Raw HTTP path**: spin up `httptest.NewServer`, verify request path/method/headers

### Integration Tests
- Uses testcontainers-go for Portainer instances
- `tests/integration/helpers/test_env.go` provides utilities
- Compare MCP handler results with direct API calls (ground-truth)
- Use `env.RawClient` with specific getters (`GetEdgeStackByName`, `GetUser`, etc.)
- Reference: `tests/integration/team_test.go`, `user_test.go`, `environment_test.go`

## Design Documentation

Design decisions are in `docs/design/` with naming convention `YYYYMM-N-short-description.md`:

| ID | Decision |
|----|----------|
| 202503-1 | External tools.yaml file |
| 202503-2 | Tool-based resource access (not MCP resources) |
| 202503-3 | Specific update tools vs single generic tool |
| 202504-1 | Embed tools.yaml in binary |
| 202504-2 | Strict tools.yaml versioning |
| 202504-3 | Pin to specific Portainer version |
| 202504-4 | Read-only security mode |
| 202602-1 | Local stack support via raw HTTP |

See `docs/design_summary.md` for the full index.

## Version Compatibility

- Supported Portainer version: **2.31.2** (defined in `server.go`)
- Tools file version: **v1.3** (defined in `tools.yaml`)
- Version check at startup; use `-disable-version-check` to bypass
