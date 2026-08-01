package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"elitegate/internal/model"
)

type TenantSnapshot struct {
	ProjectID     uuid.UUID                         `json:"project_id"`
	Routes        []model.Route                     `json:"routes"`
	Upstreams     []model.Upstream                  `json:"upstreams"`
	Targets       map[string][]model.UpstreamTarget `json:"targets"`
	APIKeys       []TenantAPIKeyDTO                 `json:"api_keys"`
	CustomDomains []model.CustomDomainSync          `json:"custom_domains"`
}

type TenantAPIKeyDTO struct {
	KeyHash string   `json:"key_hash"`
	Roles   []string `json:"roles"`
	Scopes  []string `json:"scopes"`
}

type ControlPlaneClient struct {
	adminURL     string
	projectID    string
	gatewayToken string
	httpClient   *http.Client
	logger       zerolog.Logger
}

func NewControlPlaneClient(adminURL, projectID, gatewayToken string, logger zerolog.Logger) *ControlPlaneClient {
	return &ControlPlaneClient{
		adminURL:     adminURL,
		projectID:    projectID,
		gatewayToken: gatewayToken,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		logger:       logger,
	}
}

func (c *ControlPlaneClient) ProjectID() string {
	if c == nil {
		return ""
	}
	return c.projectID
}

func (c *ControlPlaneClient) FetchSnapshot(ctx context.Context) (*TenantSnapshot, error) {
	url := fmt.Sprintf("%s/internal/v1/projects/%s/sync", c.adminURL, c.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create sync request: %w", err)
	}
	req.Header.Set("X-Gateway-Token", c.gatewayToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute sync request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	var snapshot TenantSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode snapshot response: %w", err)
	}
	return &snapshot, nil
}
