package containers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Malaccamaxgit/portainer-mcp-safe/internal/mcp"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/portainer/client-api-go/v2/pkg/client"
	"github.com/portainer/client-api-go/v2/pkg/client/auth"
	"github.com/portainer/client-api-go/v2/pkg/client/users"
	"github.com/portainer/client-api-go/v2/pkg/models"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	defaultPortainerImage = "portainer/portainer-ee:" + mcp.SupportedPortainerVersion
	defaultAPIPortTCP     = "9443/tcp"
	adminPassword         = "$2y$05$CiHrhW6R6whDVlu7Wdgl0eccb3rg1NWl/mMiO93vQiRIF1SHNFRsS" // Bcrypt hash of "adminpassword123"
	// Default ceiling for both the log-probe and the HTTPS-status-probe wait
	// strategies. Portainer EE 2.31.x typically needs 8-15s on first boot to
	// answer /api/system/status with 200 OK; the previous 5s ceiling was too
	// tight on cold runs and produced flaky "context deadline exceeded"
	// failures in every single integration test. Override at runtime with the
	// PORTAINER_TEST_STARTUP_TIMEOUT environment variable (any value
	// time.ParseDuration accepts, e.g. "90s", "2m").
	defaultStartupTimeout         = 60 * time.Second
	startupTimeoutEnvironmentName = "PORTAINER_TEST_STARTUP_TIMEOUT"
)

func startupTimeout() time.Duration {
	value := os.Getenv(startupTimeoutEnvironmentName)
	if value == "" {
		return defaultStartupTimeout
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return defaultStartupTimeout
	}
	return parsed
}

// PortainerContainer represents a Portainer container for testing
type PortainerContainer struct {
	testcontainers.Container
	APIPort  nat.Port
	APIHost  string
	apiToken string
}

// portainerContainerConfig holds the configuration for creating a Portainer container
type portainerContainerConfig struct {
	Image            string
	BindDockerSocket bool
}

// PortainerContainerOption defines a function type for applying options to Portainer container configuration
type PortainerContainerOption func(*portainerContainerConfig)

// WithImage sets a custom Portainer image
func WithImage(image string) PortainerContainerOption {
	return func(cfg *portainerContainerConfig) {
		cfg.Image = image
	}
}

// WithDockerSocketBind configures the container to bind mount the Docker socket
func WithDockerSocketBind(bind bool) PortainerContainerOption {
	return func(cfg *portainerContainerConfig) {
		cfg.BindDockerSocket = bind
	}
}

// NewPortainerContainer creates and starts a new Portainer container with the specified options
func NewPortainerContainer(ctx context.Context, opts ...PortainerContainerOption) (*PortainerContainer, error) {
	// Default configuration
	cfg := &portainerContainerConfig{
		Image:            defaultPortainerImage,
		BindDockerSocket: false,
	}

	// Apply provided options
	for _, opt := range opts {
		opt(cfg)
	}

	// Container request configuration
	req := testcontainers.ContainerRequest{
		Image:        cfg.Image,
		ExposedPorts: []string{defaultAPIPortTCP},
		WaitingFor: wait.ForAll(
			wait.ForLog("starting HTTPS server").
				WithStartupTimeout(startupTimeout()),
			wait.ForHTTP("/api/system/status").
				WithTLS(true, nil).
				WithAllowInsecure(true).
				WithPort(defaultAPIPortTCP).
				WithStatusCodeMatcher(
					func(status int) bool {
						return status == http.StatusOK
					},
				).
				WithStartupTimeout(startupTimeout()),
		),
		Cmd: []string{
			"--admin-password",
			adminPassword,
			"--log-level",
			"DEBUG",
		},
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			if cfg.BindDockerSocket {
				hostConfig.Binds = append(hostConfig.Binds, "/var/run/docker.sock:/var/run/docker.sock")
			}
		},
	}

	// Create and start the container
	cntr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start Portainer container: %w", err)
	}

	// Get the host and port mapping
	host, err := cntr.Host(ctx)
	if err != nil {
		_ = cntr.Terminate(ctx) // Clean up if we fail post-start
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	mappedPort, err := cntr.MappedPort(ctx, nat.Port(defaultAPIPortTCP))
	if err != nil {
		_ = cntr.Terminate(ctx) // Clean up if we fail post-start
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	pc := &PortainerContainer{
		Container: cntr,
		APIPort:   mappedPort,
		APIHost:   host,
	}

	// Register API token after successful container start and port mapping
	if err := pc.registerAPIToken(); err != nil {
		// Attempt to clean up the container if token registration fails
		_ = cntr.Terminate(ctx)
		return nil, fmt.Errorf("failed to register API token: %w", err)
	}

	return pc, nil
}

// GetAPIBaseURL returns the base URL for the Portainer API
func (pc *PortainerContainer) GetAPIBaseURL() string {
	return fmt.Sprintf("https://%s:%s", pc.APIHost, pc.APIPort.Port())
}

// GetHostAndPort returns the host and port for the Portainer API
func (pc *PortainerContainer) GetHostAndPort() (string, string) {
	return pc.APIHost, pc.APIPort.Port()
}

func (pc *PortainerContainer) GetAPIToken() string {
	return pc.apiToken
}

// registerAPIToken registers an API token for the admin user
func (pc *PortainerContainer) registerAPIToken() error {
	transport := httptransport.New(
		fmt.Sprintf("%s:%s", pc.APIHost, pc.APIPort.Port()),
		"/api",
		[]string{"https"},
	)

	transport.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	portainerClient := client.New(transport, strfmt.Default)

	username := "admin"
	password := "adminpassword123"
	params := auth.NewAuthenticateUserParams().WithBody(&models.AuthAuthenticatePayload{
		Username: &username,
		Password: &password,
	})

	authResp, err := portainerClient.Auth.AuthenticateUser(params)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	token := authResp.Payload.Jwt

	// Setup JWT authentication
	jwtAuth := runtime.ClientAuthInfoWriterFunc(func(r runtime.ClientRequest, _ strfmt.Registry) error {
		return r.SetHeaderParam("Authorization", fmt.Sprintf("Bearer %s", token))
	})
	transport.DefaultAuthentication = jwtAuth

	description := "test-api-key"
	createTokenParams := users.NewUserGenerateAPIKeyParams().WithID(1).WithBody(&models.UsersUserAccessTokenCreatePayload{
		Description: &description,
		Password:    &password,
	})

	createTokenResp, err := portainerClient.Users.UserGenerateAPIKey(createTokenParams, nil)
	if err != nil {
		return fmt.Errorf("failed to generate API key: %w", err)
	}

	pc.apiToken = createTokenResp.Payload.RawAPIKey

	return nil
}
