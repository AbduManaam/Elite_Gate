package service

import (
	"context"
	"fmt"

	"elitegate/internal/ipfilter"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/rs/zerolog"
)

type PolicyService struct {
	policyRepo *storage.PolicyRepo
	routeRepo  *storage.RouteRepo
	logger     zerolog.Logger
}

func NewPolicyService(policyRepo *storage.PolicyRepo, routeRepo *storage.RouteRepo, logger zerolog.Logger) *PolicyService {
	return &PolicyService{
		policyRepo: policyRepo,
		routeRepo:  routeRepo,
		logger:     logger.With().Str("service", "policy").Logger(),
	}
}

func ValidateIPRules(field string, ips []string) error {
	if len(ips) == 0 {
		return nil
	}
	if _, err := ipfilter.NewIPChecker(ips); err != nil {
		return fmt.Errorf("invalid IP or CIDR in %s: %w", field, err)
	}
	return nil
}

func (s *PolicyService) ListPolicies(ctx context.Context, limit, offset int) ([]model.Policy, int, error) {
	policies, total, err := s.policyRepo.ListAll(ctx, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list policies: %w", err)
	}
	return policies, total, nil
}

func (s *PolicyService) CreatePolicy(ctx context.Context, name string, authRequired bool, rpm int, origins, roles, scopes, ipAllowlist, ipBlocklist []string) (*model.Policy, error) {
	if rpm < 0 {
		return nil, fmt.Errorf("rate_limit_rpm must be >= 0")
	}

	if err := ValidateIPRules("ip_allowlist", ipAllowlist); err != nil {
		return nil, err
	}
	if err := ValidateIPRules("ip_blocklist", ipBlocklist); err != nil {
		return nil, err
	}

	p := &model.Policy{
		Name:           name,
		AuthRequired:   authRequired,
		RateLimitRPM:   rpm,
		AllowedOrigins: origins,
		AllowedRoles:   roles,
		AllowedScopes:  scopes,
		IPAllowlist:    ipAllowlist,
		IPBlocklist:    ipBlocklist,
	}

	if err := s.policyRepo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create policy: %w", err)
	}

	return p, nil
}

func (s *PolicyService) UpdatePolicy(ctx context.Context, id, name string, authRequired bool, rpm int, origins, roles, scopes, ipAllowlist, ipBlocklist []string) (*model.Policy, error) {
	if rpm < 0 {
		return nil, fmt.Errorf("rate_limit_rpm must be >= 0")
	}

	if err := ValidateIPRules("ip_allowlist", ipAllowlist); err != nil {
		return nil, err
	}
	if err := ValidateIPRules("ip_blocklist", ipBlocklist); err != nil {
		return nil, err
	}

	p := &model.Policy{
		Name:           name,
		AuthRequired:   authRequired,
		RateLimitRPM:   rpm,
		AllowedOrigins: origins,
		AllowedRoles:   roles,
		AllowedScopes:  scopes,
		IPAllowlist:    ipAllowlist,
		IPBlocklist:    ipBlocklist,
	}

	if err := s.policyRepo.Update(ctx, id, p); err != nil {
		return nil, fmt.Errorf("update policy: %w", err)
	}

	return p, nil
}

func (s *PolicyService) DeletePolicy(ctx context.Context, id string) error {
	if err := s.policyRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	return nil
}

func (s *PolicyService) AssignPolicy(ctx context.Context, routeID, policyID string) error {
	if err := s.routeRepo.AssignPolicy(ctx, routeID, policyID); err != nil {
		return fmt.Errorf("assign policy: %w", err)
	}
	return nil
}

func (s *PolicyService) RemovePolicy(ctx context.Context, routeID string) error {
	if err := s.routeRepo.RemovePolicy(ctx, routeID); err != nil {
		return fmt.Errorf("remove policy: %w", err)
	}
	return nil
}
