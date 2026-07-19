package service

import (
	"context"
	"fmt"

	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

type PlatformService struct {
	projectRepo *storage.ProjectRepo
	gatewayRepo *storage.GatewayRepo
	authRepo    *storage.AdminAuthRepo
	logger      zerolog.Logger
}

func NewPlatformService(
	projectRepo *storage.ProjectRepo,
	gatewayRepo *storage.GatewayRepo,
	authRepo *storage.AdminAuthRepo,
	logger zerolog.Logger,
) *PlatformService {
	return &PlatformService{
		projectRepo: projectRepo,
		gatewayRepo: gatewayRepo,
		authRepo:    authRepo,
		logger:      logger.With().Str("service", "platform").Logger(),
	}
}

func (s *PlatformService) ListTenants(ctx context.Context, limit, offset int) ([]model.Project, int, error) {
	projects, total, err := s.projectRepo.ListAllGlobal(ctx, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenants: %w", err)
	}
	return projects, total, nil
}

func (s *PlatformService) DeleteTenant(ctx context.Context, projectID string) error {
	if err := s.projectRepo.Delete(ctx, projectID); err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	return nil
}

func (s *PlatformService) GetPlatformHealthCounts(ctx context.Context) (storage.ProjectCounts, map[string]int, error) {
	projectCounts, err := s.projectRepo.GlobalCounts(ctx)
	if err != nil {
		return storage.ProjectCounts{}, nil, fmt.Errorf("project counts: %w", err)
	}

	gatewayCounts, err := s.gatewayRepo.CountByStatus(ctx)
	if err != nil {
		return storage.ProjectCounts{}, nil, fmt.Errorf("gateway counts: %w", err)
	}

	return projectCounts, gatewayCounts, nil
}

func (s *PlatformService) GetPlatformMetricsCounts(ctx context.Context) (storage.ProjectCounts, map[string]int, int, error) {
	projectCounts, err := s.projectRepo.GlobalCounts(ctx)
	if err != nil {
		return storage.ProjectCounts{}, nil, 0, fmt.Errorf("project counts: %w", err)
	}

	gatewayCounts, err := s.gatewayRepo.CountByStatus(ctx)
	if err != nil {
		return storage.ProjectCounts{}, nil, 0, fmt.Errorf("gateway counts: %w", err)
	}

	adminUserCount, err := s.authRepo.AdminUserCount(ctx)
	if err != nil {
		return storage.ProjectCounts{}, nil, 0, fmt.Errorf("admin user count: %w", err)
	}

	return projectCounts, gatewayCounts, adminUserCount, nil
}

func (s *PlatformService) SuspendTenant(ctx context.Context, projectID string) error {
	if err := s.projectRepo.Suspend(ctx, projectID); err != nil {
		return fmt.Errorf("suspend tenant: %w", err)
	}
	return nil
}

func (s *PlatformService) ReactivateTenant(ctx context.Context, projectID string) error {
	if err := s.projectRepo.Reactivate(ctx, projectID); err != nil {
		return fmt.Errorf("reactivate tenant: %w", err)
	}
	return nil
}
