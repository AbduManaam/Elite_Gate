package service

import (
	"context"
	"errors"
	"fmt"

	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

var ErrInvalidMatchType = errors.New("match_type must be 'exact' or 'prefix'")
var validMatchTypes = map[string]bool{"exact": true, "prefix": true}

type RouteService struct {
	repo   *storage.RouteRepo
	logger zerolog.Logger
}

func NewRouteService(repo *storage.RouteRepo, logger zerolog.Logger) *RouteService {
	return &RouteService{
		repo:   repo,
		logger: logger.With().Str("service", "route").Logger(),
	}
}

func (s *RouteService) ListRoutes(ctx context.Context, limit, offset int) ([]model.Route, int, error) {
	routes, total, err := s.repo.ListAll(ctx, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list routes: %w", err)
	}
	return routes, total, nil
}

func (s *RouteService) CreateRoute(ctx context.Context, name, pathStr, upstreamID string, policyID *string, methods []string, matchType string, enabled bool) (*model.Route, error) {
	if matchType == "" {
		matchType = "prefix"
	}
	if !validMatchTypes[matchType] {
		return nil, ErrInvalidMatchType
	}

	var pid *string
	if policyID != nil && *policyID != "" {
		pid = policyID
	}

	rt := &model.Route{
		Name:       name,
		Path:       pathStr,
		UpstreamID: &upstreamID,
		PolicyID:   pid,
		Methods:    methods,
		MatchType:  matchType,
		Enabled:    enabled,
	}

	if err := s.repo.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("create route: %w", err)
	}

	return rt, nil
}

func (s *RouteService) DeleteRoute(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete route: %w", err)
	}
	return nil
}

func (s *RouteService) UpdateRoute(ctx context.Context, id, name, pathStr, upstreamID string, policyID *string, methods []string, matchType string, enabled bool) (*model.Route, error) {
	if matchType == "" {
		matchType = "prefix"
	}
	if !validMatchTypes[matchType] {
		return nil, ErrInvalidMatchType
	}

	var pid *string
	if policyID != nil && *policyID != "" {
		pid = policyID
	}

	rt := &model.Route{
		Name:       name,
		Path:       pathStr,
		UpstreamID: &upstreamID,
		PolicyID:   pid,
		Methods:    methods,
		MatchType:  matchType,
		Enabled:    enabled,
	}

	if err := s.repo.Update(ctx, id, rt); err != nil {
		return nil, fmt.Errorf("update route: %w", err)
	}

	return rt, nil
}

func (s *RouteService) DisableRoute(ctx context.Context, id string) error {
	if err := s.repo.Disable(ctx, id); err != nil {
		return fmt.Errorf("disable route: %w", err)
	}
	return nil
}

func (s *RouteService) EnableRoute(ctx context.Context, id string) error {
	if err := s.repo.Enable(ctx, id); err != nil {
		return fmt.Errorf("enable route: %w", err)
	}
	return nil
}
