package service

import (
	"context"
	"errors"
	"fmt"

	"elitegate/internal/storage"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var ErrInvalidMemberRole = errors.New("role must be one of: owner, editor, viewer")
var validMemberRoles = map[string]bool{"owner": true, "editor": true, "viewer": true}

type MembershipService struct {
	repo   *storage.MembershipRepo
	logger zerolog.Logger
}

func NewMembershipService(repo *storage.MembershipRepo, logger zerolog.Logger) *MembershipService {
	return &MembershipService{
		repo:   repo,
		logger: logger.With().Str("service", "membership").Logger(),
	}
}

func (s *MembershipService) AddMember(ctx context.Context, projectID uuid.UUID, email, role string, inviterID uuid.UUID) (*storage.ProjectMember, error) {
	if !validMemberRoles[role] {
		return nil, ErrInvalidMemberRole
	}

	target, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("find user by email: %w", err)
	}

	if err := s.repo.AddMember(ctx, projectID, target.ID, role, inviterID); err != nil {
		return nil, fmt.Errorf("add member: %w", err)
	}

	return &storage.ProjectMember{
		ProjectID:   projectID,
		AdminUserID: target.ID,
		Username:    target.Username,
		Email:       target.Email,
		Role:        role,
	}, nil
}

func (s *MembershipService) LookupMemberByEmail(ctx context.Context, email string) (*storage.MemberLookupResult, error) {
	target, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("lookup member by email: %w", err)
	}
	return &target, nil
}

func (s *MembershipService) ChangeRole(ctx context.Context, projectID, targetUserID uuid.UUID, newRole string) error {
	if !validMemberRoles[newRole] {
		return ErrInvalidMemberRole
	}

	if err := s.repo.UpdateRole(ctx, projectID, targetUserID, newRole); err != nil {
		return fmt.Errorf("change member role: %w", err)
	}

	return nil
}

func (s *MembershipService) RemoveMember(ctx context.Context, projectID, targetUserID uuid.UUID) error {
	if err := s.repo.RemoveMember(ctx, projectID, targetUserID); err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	return nil
}

func (s *MembershipService) ListMembers(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]storage.ProjectMember, int, error) {
	members, total, err := s.repo.ListMembers(ctx, projectID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list members: %w", err)
	}
	return members, total, nil
}
