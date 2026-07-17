package container

// Uses the " Docker Go SDK " instead of Docker CLI commands.
// Faster and easier to manage Docker programmatically.
import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	gatewayNetworkName  = "elitegate_net"
	gatewayLabelPrefix  = "elitegate.gateway.id"
	gatewayProjectLabel = "elitegate.project.id"

	containerStartTimeout = 30 * time.Second
	healthPollInterval    = 500 * time.Millisecond
)

// Gateway's internal container port.
// Shared by Provision and inspectEndpoint.
var gatewayContainerPort = nat.Port("8080/tcp")

// Allows only valid characters in gateway IDs.
// Prevents Docker container name errors.
var gatewayIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)

// Manages creation and removal of isolated gateway containers using Docker API.
type DockerContainerManager struct {
	client      *client.Client
	postgresDSN string
	redisAddr   string
	jwtSecret   string
	imageName   string
	networkName string
	logger      zerolog.Logger
}

// Creates a Docker container manager connected to the Docker daemon.
// The caller must call Close() when the manager is no longer needed.
func NewDockerContainerManager(postgresDSN, redisAddr, jwtSecret, imageName string) (*DockerContainerManager, error) {
	// Ensure logs directory exists.
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logFileWriter := &lumberjack.Logger{
		Filename:   "logs/container.log",
		MaxSize:    10, // megabytes
		MaxBackups: 5,
		MaxAge:     30, // days
		Compress:   true,
	}

	multi := zerolog.MultiLevelWriter(
		logFileWriter,
		zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339},
	)

	logger := zerolog.New(multi).With().Timestamp().Str("component", "container-manager").Logger()

	logger.Info().Msg("initializing docker container manager")

	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		logger.Error().Err(err).Msg("failed to connect to docker daemon")
		return nil, fmt.Errorf("connect to docker daemon: %w", err)
	}

	if imageName == "" {
		imageName = "elitegate-gateway:latest"
	}

	m := &DockerContainerManager{
		client:      cli,
		postgresDSN: postgresDSN,
		redisAddr:   redisAddr,
		jwtSecret:   jwtSecret,
		imageName:   imageName,
		networkName: gatewayNetworkName,
		logger:      logger,
	}

	if err := m.ensureNetwork(context.Background()); err != nil {
		logger.Error().Err(err).Msg("failed to ensure docker network")
		_ = cli.Close()
		return nil, fmt.Errorf("ensure docker network: %w", err)
	}

	logger.Info().
		Str("image_name", imageName).
		Str("network_name", m.networkName).
		Msg("docker container manager initialized successfully")

	return m, nil
}

func (m *DockerContainerManager) Close() error {
	m.logger.Info().Msg("closing docker container manager connection")
	return m.client.Close()
}

// Provision: Creates and starts a new gateway container for a project.
// Waits until the container is healthy.
// Returns the internal endpoint (for admin health checks / config reloads) and
// the public endpoint (localhost:<docker-port>) that external clients must use.
func (m *DockerContainerManager) Provision(ctx context.Context, gatewayID, projectID, plan string) (endpointIP, gatewayPort, publicHost, publicPort string, err error) {
	m.logger.Info().
		Str("gateway_id", gatewayID).
		Str("project_id", projectID).
		Str("plan", plan).
		Msg("provisioning gateway container")

	// Ensure the gateway ID is valid for use as a Docker container name.
	if !gatewayIDPattern.MatchString(gatewayID) {
		err = fmt.Errorf("invalid gatewayID %q: must match %s", gatewayID, gatewayIDPattern)
		m.logger.Error().Err(err).Msg("invalid gateway ID")
		return "", "", "", "", err
	}

	containerName := fmt.Sprintf("elitegate-gateway-%s", gatewayID)

	// Let Docker automatically choose an available port.
	portBindings := nat.PortMap{
		gatewayContainerPort: []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: ""},
		},
	}
	exposedPorts := nat.PortSet{gatewayContainerPort: struct{}{}}

	cfg := &container.Config{
		Image: m.imageName,
		Env: []string{
			"GATEWAY_PORT=8080",
			fmt.Sprintf("POSTGRES_DSN=%s", m.postgresDSN),
			fmt.Sprintf("REDIS_ADDR=%s", m.redisAddr),
			fmt.Sprintf("JWT_SECRET=%s", m.jwtSecret),
			fmt.Sprintf("PROJECT_ID=%s", projectID),
			"APP_ENV=production",
			"ROUTE_RELOAD_INTERVAL=10s",
		},
		ExposedPorts: exposedPorts,
		Labels: map[string]string{
			gatewayLabelPrefix:  gatewayID,
			gatewayProjectLabel: projectID,
			"elitegate.plan":    plan,
		},
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD", "wget", "-qO-", "http://localhost:8080/health"},
			Interval: 2 * time.Second,
			Timeout:  3 * time.Second,
			Retries:  5,
		},
	}

	hostCfg := &container.HostConfig{
		PortBindings: portBindings,
		RestartPolicy: container.RestartPolicy{
			Name:              "on-failure",
			MaximumRetryCount: 3,
		},
		Resources: container.Resources{
			Memory:   256 * 1024 * 1024, // 256 MB hard limit
			NanoCPUs: 500_000_000,       // 0.5 vCPU
		},
	}

	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			m.networkName: {},
		},
	}

	m.logger.Debug().Str("container_name", containerName).Msg("creating container")
	// Create the container (does not start it).
	resp, err := m.client.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, containerName)
	if err != nil {
		m.logger.Error().Err(err).Str("container_name", containerName).Msg("failed to create container")
		return "", "", "", "", fmt.Errorf("create container %q: %w", containerName, err)
	}

	m.logger.Debug().Str("container_id", resp.ID).Msg("starting container")
	// Start the container.
	if err := m.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		m.logger.Error().Err(err).Str("container_id", resp.ID).Msg("failed to start container, cleaning up")
		// Best-effort cleanup — ignore secondary error.
		_ = m.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", "", "", "", fmt.Errorf("start container %q: %w", containerName, err)
	}

	m.logger.Debug().Str("container_id", resp.ID).Msg("waiting for container to become healthy")
	// Wait until the healthcheck passes (or deadline exceeded).
	if err := m.waitUntilHealthy(ctx, resp.ID, containerStartTimeout); err != nil {
		m.logger.Error().Err(err).Str("container_id", resp.ID).Msg("container failed health check, retrieving logs and cleaning up")
		// Capture logs before removing so the caller can diagnose startup failures.
		logs := m.lastLogs(context.Background(), resp.ID, 20)
		_ = m.client.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
		return "", "", "", "", fmt.Errorf("container %q did not become healthy: %w\nlast logs:\n%s", containerName, err, logs)
	}

	m.logger.Debug().Str("container_id", resp.ID).Msg("inspecting container endpoint settings")
	// Read the actual bound port and container IP from the Docker daemon.
	// This is the only reliable source — reading before Start gives stale data.
	ip, port, pubHost, pubPort, err := m.inspectEndpoint(ctx, resp.ID)
	if err != nil {
		m.logger.Error().Err(err).Str("container_id", resp.ID).Msg("failed to inspect container endpoint, cleaning up")
		_ = m.client.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
		return "", "", "", "", fmt.Errorf("read container endpoint: %w", err)
	}

	m.logger.Info().
		Str("gateway_id", gatewayID).
		Str("container_id", resp.ID).
		Str("ip", ip).
		Str("port", port).
		Str("public_host", pubHost).
		Str("public_port", pubPort).
		Msg("gateway container provisioned successfully")

	return ip, port, pubHost, pubPort, nil
}

// inspectEndpoint returns two distinct addresses:
//   - internalHost/internalPort: how the admin process itself should reach the
//     gateway (Docker DNS name when admin runs in-container, else localhost).
//     Used for health checks and config-reload calls (sync_handler.go, platform_handler.go).
//   - publicHost/publicPort: the host-mapped address an external client
//     (browser, curl) must use. This is ALWAYS localhost:<docker-assigned-port>,
//     regardless of where the admin process itself is running.
func (m *DockerContainerManager) inspectEndpoint(ctx context.Context, containerID string) (internalHost, internalPort, publicHost, publicPort string, err error) {
	info, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", "", "", "", fmt.Errorf("inspect container: %w", err)
	}

	// Get the actual host port assigned by Docker.
	bindings, ok := info.NetworkSettings.Ports[gatewayContainerPort]
	if !ok || len(bindings) == 0 {
		return "", "", "", "", fmt.Errorf("no port binding found for %s", gatewayContainerPort)
	}
	hostPort := bindings[0].HostPort
	// This is the one users need to actually call the API with, e.g. localhost:50979.
	publicHost, publicPort = "localhost", hostPort

	// Get the container's IP address from the shared network.
	// Use the default bridge network IP if needed.
	containerIP := info.NetworkSettings.IPAddress // default bridge
	if ep, ok := info.NetworkSettings.Networks[m.networkName]; ok && ep.IPAddress != "" {
		containerIP = ep.IPAddress
	}
	if containerIP == "" {
		return "", "", "", "", fmt.Errorf("container has no IP address on network %q", m.networkName)
	}

	if runningInContainer() {
		// Use stable container name for Docker DNS resolution
		containerName := strings.TrimPrefix(info.Name, "/")
		return containerName, gatewayContainerPort.Port(), publicHost, publicPort, nil
	}

	return "localhost", hostPort, publicHost, publicPort, nil
}

func runningInContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}

// Decommission: Stops and removes a gateway container.
// Returns success even if the container is already removed.
func (m *DockerContainerManager) Decommission(ctx context.Context, gatewayID string) error {
	m.logger.Info().Str("gateway_id", gatewayID).Msg("decommissioning gateway container")

	if !gatewayIDPattern.MatchString(gatewayID) {
		err := fmt.Errorf("invalid gatewayID %q: must match %s", gatewayID, gatewayIDPattern)
		m.logger.Error().Err(err).Msg("invalid gateway ID for decommission")
		return err
	}

	containerName := fmt.Sprintf("elitegate-gateway-%s", gatewayID)

	m.logger.Debug().Str("container_name", containerName).Msg("listing containers matching name")
	containers, err := m.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", containerName)),
	})
	if err != nil {
		m.logger.Error().Err(err).Str("container_name", containerName).Msg("failed to list containers")
		return fmt.Errorf("list containers: %w", err)
	}

	if len(containers) == 0 {
		m.logger.Info().Str("container_name", containerName).Msg("no matching container found, already decommissioned")
		return nil
	}

	m.logger.Debug().Str("container_id", containers[0].ID).Msg("stopping container gracefully")
	//Attempts to gracefully stop the container, giving it time to shut down cleanly before being forcefully removed.
	timeout := 10
	_ = m.client.ContainerStop(ctx, containers[0].ID, container.StopOptions{Timeout: &timeout})

	m.logger.Debug().Str("container_id", containers[0].ID).Msg("removing container")
	// Remove the container using Docker v28 RemoveOptions.
	err = m.client.ContainerRemove(ctx, containers[0].ID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: false,
	})
	if err != nil {
		m.logger.Error().Err(err).Str("container_id", containers[0].ID).Msg("failed to remove container")
		return err
	}

	m.logger.Info().Str("gateway_id", gatewayID).Msg("gateway container decommissioned successfully")
	return nil
}

// Ensure the shared Docker network exists.Creates it if it doesn't already exist.
func (m *DockerContainerManager) ensureNetwork(ctx context.Context) error {
	m.logger.Debug().Str("network_name", m.networkName).Msg("checking if docker network exists")

	//List Docker networks.
	networks, err := m.client.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", m.networkName)),
	})
	if err != nil {
		m.logger.Error().Err(err).Str("network_name", m.networkName).Msg("failed to list networks")
		return fmt.Errorf("list networks: %w", err)
	}

	if len(networks) > 0 {
		m.logger.Debug().Str("network_name", m.networkName).Msg("docker network already exists")
		return nil // Network already exists.
	}

	m.logger.Info().Str("network_name", m.networkName).Msg("docker network not found, creating it")
	//Create a network using network.CreateOptions.
	_, err = m.client.NetworkCreate(ctx, m.networkName, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{"elitegate.managed": "true"},
	})
	if err != nil {
		m.logger.Error().Err(err).Str("network_name", m.networkName).Msg("failed to create network")
		return fmt.Errorf("create network %q: %w", m.networkName, err)
	}

	m.logger.Info().Str("network_name", m.networkName).Msg("docker network created successfully")
	return nil
}

// Wait until the container is healthy or the timeout is reached.
func (m *DockerContainerManager) waitUntilHealthy(ctx context.Context, containerID string, timeout time.Duration) error {
	m.logger.Debug().Str("container_id", containerID).Msg("waiting for container health check")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		inspect, err := m.client.ContainerInspect(ctx, containerID)
		if err != nil {
			m.logger.Error().Err(err).Str("container_id", containerID).Msg("inspect failed while waiting for health check")
			return fmt.Errorf("inspect container: %w", err)
		}

		if !inspect.State.Running {
			err = fmt.Errorf("container exited unexpectedly with code %d", inspect.State.ExitCode)
			m.logger.Error().Err(err).Str("container_id", containerID).Msg("container is not running")
			return err
		}

		//// If no health check exists, a running container is considered healthy.
		if inspect.State.Health == nil {
			m.logger.Debug().Str("container_id", containerID).Msg("no health check defined, container is running and considered healthy")
			return nil
		}

		m.logger.Debug().
			Str("container_id", containerID).
			Str("health_status", inspect.State.Health.Status).
			Msg("polled container health status")

		switch inspect.State.Health.Status {
		case "healthy":
			return nil
		case "unhealthy":
			err = fmt.Errorf("container healthcheck reported unhealthy")
			m.logger.Error().Err(err).Str("container_id", containerID).Msg("health check failed")
			return err
		}

		select {
		case <-ctx.Done():
			m.logger.Error().Err(ctx.Err()).Str("container_id", containerID).Msg("context cancelled while waiting for health check")
			return ctx.Err()
		case <-time.After(healthPollInterval):
		}
	}

	err := fmt.Errorf("container did not become healthy within %s", timeout)
	m.logger.Error().Err(err).Str("container_id", containerID).Msg("health check timed out")
	return err
}

// Get the last container logs.Used to help diagnose startup failures.
func (m *DockerContainerManager) lastLogs(ctx context.Context, containerID string, tail int) string {
	m.logger.Debug().Str("container_id", containerID).Int("tail", tail).Msg("fetching container logs")
	//Get container logs using container.LogsOptions.
	rc, err := m.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
	})
	if err != nil {
		m.logger.Error().Err(err).Str("container_id", containerID).Msg("failed to get container logs")
		return "(could not retrieve logs)"
	}
	defer rc.Close()

	b, _ := io.ReadAll(rc)
	return string(b)
}
