package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"elitegate/internal/admin/middleware"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

type ProjectService struct {
	repo         *storage.ProjectRepo
	originCache  *middleware.OriginCache
	summaryCache *storage.SummaryCache
	logger       zerolog.Logger
}

func NewProjectService(repo *storage.ProjectRepo, originCache *middleware.OriginCache, logger zerolog.Logger) *ProjectService {
	return &ProjectService{
		repo:         repo,
		originCache:  originCache,
		summaryCache: storage.NewSummaryCache(10 * time.Second),
		logger:       logger.With().Str("service", "project").Logger(),
	}
}

func (s *ProjectService) CreateProject(ctx context.Context, name, slug, description, ownerID, plan string) (*model.Project, error) {
	if plan == "" {
		plan = "free"
	}

	p := &model.Project{
		Name:        name,
		Slug:        strings.ToLower(slug),
		Description: description,
		OwnerID:     ownerID,
		Plan:        plan,
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	return p, nil
}

func (s *ProjectService) ListProjects(ctx context.Context, userID string, limit, offset int) ([]model.Project, int, error) {
	projects, total, err := s.repo.ListForUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list projects: %w", err)
	}
	return projects, total, nil
}

func (s *ProjectService) UpdateProject(ctx context.Context, id, name, description, plan string) (*model.Project, error) {
	p := &model.Project{
		Name:        name,
		Description: description,
		Plan:        plan,
	}

	if err := s.repo.Update(ctx, id, p); err != nil {
		return nil, fmt.Errorf("update project: %w", err)
	}

	return p, nil
}

func (s *ProjectService) DeleteProject(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

func (s *ProjectService) GetProjectSummary(ctx context.Context, projectID, userRole string) (*model.ProjectSummary, error) {
	if cached, ok := s.summaryCache.Get(projectID); ok {
		cloned := *cached
		s.ApplyRoleBasedFields(&cloned, userRole, projectID)
		cloned.Role = userRole
		return &cloned, nil
	}

	summary, err := s.repo.GetSummary(ctx)
	if err != nil {
		return nil, fmt.Errorf("get project summary: %w", err)
	}

	s.summaryCache.Set(projectID, summary)

	cloned := *summary
	s.ApplyRoleBasedFields(&cloned, userRole, projectID)
	cloned.Role = userRole

	return &cloned, nil
}

func (s *ProjectService) ApplyRoleBasedFields(summary *model.ProjectSummary, role string, projectID string) {
	if role == "owner" {
		status := "active"
		if !summary.IsActive {
			status = "suspended"
		}
		summary.Subscription = &model.Subscription{
			Plan:   *summary.Plan,
			Status: status,
		}
	} else {
		summary.Plan = nil
		summary.Subscription = nil
	}
}

func (s *ProjectService) UpdateDashboardOrigins(ctx context.Context, projectID string, origins []string) ([]string, error) {
	if err := s.repo.UpdateDashboardOrigins(ctx, projectID, origins); err != nil {
		return nil, fmt.Errorf("update dashboard origins: %w", err)
	}

	if s.originCache != nil {
		s.originCache.Invalidate(projectID)
	}

	return origins, nil
}
