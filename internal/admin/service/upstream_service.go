package service

import (
	"context"
	"errors"
	"fmt"

	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

var (
	ErrInvalidProtocol   = errors.New("protocol must be 'http' or 'grpc'")
	ErrInvalidLBStrategy = errors.New("lb_strategy must be 'round_robin' or 'least_conn'")

	validProtocols   = map[string]bool{"http": true, "grpc": true}
	validLBStrategies = map[string]bool{"round_robin": true, "least_conn": true}
)

type UpstreamService struct {
	upstreamRepo *storage.UpstreamRepo
	targetRepo   *storage.UpstreamTargetRepo
	logger       zerolog.Logger
}

func NewUpstreamService(upstreamRepo *storage.UpstreamRepo, targetRepo *storage.UpstreamTargetRepo, logger zerolog.Logger) *UpstreamService {
	return &UpstreamService{
		upstreamRepo: upstreamRepo,
		targetRepo:   targetRepo,
		logger:       logger.With().Str("service", "upstream").Logger(),
	}
}

func (s *UpstreamService) ListUpstreams(ctx context.Context, limit, offset int) ([]model.Upstream, int, error) {
	upstreams, total, err := s.upstreamRepo.ListAll(ctx, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list upstreams: %w", err)
	}
	return upstreams, total, nil
}

func (s *UpstreamService) CreateUpstream(ctx context.Context, name, targetURL, protocol, healthPath, lbStrategy string, enabled bool) (*model.Upstream, error) {
	if !validProtocols[protocol] {
		return nil, ErrInvalidProtocol
	}

	if lbStrategy == "" {
		lbStrategy = "round_robin"
	}
	if !validLBStrategies[lbStrategy] {
		return nil, ErrInvalidLBStrategy
	}

	u := &model.Upstream{
		Name:       name,
		TargetURL:  targetURL,
		Protocol:   protocol,
		HealthPath: healthPath,
		Enabled:    enabled,
		LBStrategy: lbStrategy,
	}

	if err := s.upstreamRepo.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("create upstream: %w", err)
	}

	return u, nil
}

func (s *UpstreamService) UpdateUpstream(ctx context.Context, id, name, targetURL, protocol, healthPath, lbStrategy string, enabled bool) (*model.Upstream, error) {
	if !validProtocols[protocol] {
		return nil, ErrInvalidProtocol
	}

	if lbStrategy == "" {
		lbStrategy = "round_robin"
	}
	if !validLBStrategies[lbStrategy] {
		return nil, ErrInvalidLBStrategy
	}

	u := &model.Upstream{
		Name:       name,
		TargetURL:  targetURL,
		Protocol:   protocol,
		HealthPath: healthPath,
		Enabled:    enabled,
		LBStrategy: lbStrategy,
	}

	if err := s.upstreamRepo.Update(ctx, id, u); err != nil {
		return nil, fmt.Errorf("update upstream: %w", err)
	}

	return u, nil
}

func (s *UpstreamService) DisableUpstream(ctx context.Context, id string) error {
	if err := s.upstreamRepo.Disable(ctx, id); err != nil {
		return fmt.Errorf("disable upstream: %w", err)
	}
	return nil
}

func (s *UpstreamService) DeleteUpstream(ctx context.Context, id string) error {
	if err := s.upstreamRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete upstream: %w", err)
	}
	return nil
}

func (s *UpstreamService) GetUpstreamByID(ctx context.Context, id string) (*model.Upstream, error) {
	u, err := s.upstreamRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get upstream by id: %w", err)
	}
	return u, nil
}

func (s *UpstreamService) AddTarget(ctx context.Context, upstreamID, targetURL string, weight int, enabled bool) (*model.UpstreamTarget, error) {
	if weight <= 0 {
		weight = 1
	}

	t := &model.UpstreamTarget{
		UpstreamID: upstreamID,
		TargetURL:  targetURL,
		Weight:     weight,
		Enabled:    enabled,
	}

	if err := s.targetRepo.Add(ctx, t); err != nil {
		return nil, fmt.Errorf("add upstream target: %w", err)
	}

	return t, nil
}

func (s *UpstreamService) ListTargets(ctx context.Context, upstreamID string) ([]model.UpstreamTarget, error) {
	targets, err := s.targetRepo.ListByUpstream(ctx, upstreamID)
	if err != nil {
		return nil, fmt.Errorf("list upstream targets: %w", err)
	}
	return targets, nil
}

func (s *UpstreamService) RemoveTarget(ctx context.Context, targetID string) error {
	if err := s.targetRepo.Remove(ctx, targetID); err != nil {
		return fmt.Errorf("remove upstream target: %w", err)
	}
	return nil
}
