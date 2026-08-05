package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"elitegate/internal/aws"
	"elitegate/internal/container"
	"elitegate/internal/storage"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const (
	gatewayStatusContainerReady         = "container_ready"
	gatewayStatusRegisteringTarget      = "registering_target"
	gatewayStatusCreatingListenerRule   = "creating_listener_rule"
	gatewayStatusWaitingForTargetHealth = "waiting_for_target_health"
	gatewayStatusDeprovisioning         = "deprovisioning"
)

type GatewayProvisioningRepository interface {
	ClaimNextGatewayIngressJob(
		ctx context.Context,
		workerID string,
		lockTimeout time.Duration,
	) (*storage.GatewayProvisioningJob, error)

	MarkGatewayTargetGroupCreated(
		ctx context.Context,
		externalID string,
		leaseToken uuid.UUID,
		targetGroupARN string,
	) error

	MarkGatewayTargetRegistered(
		ctx context.Context,
		externalID string,
		leaseToken uuid.UUID,
	) error

	ReserveGatewayListenerRulePriority(
		ctx context.Context,
		externalID string,
		leaseToken uuid.UUID,
		minPriority int,
		maxPriority int,
	) (int, error)

	MarkGatewayListenerRuleCreated(
		ctx context.Context,
		externalID string,
		leaseToken uuid.UUID,
		ruleARN string,
		nextCheckAt time.Time,
	) error

	RescheduleGatewayHealthCheck(
		ctx context.Context,
		externalID string,
		leaseToken uuid.UUID,
		nextCheckAt time.Time,
	) error

	MarkGatewayIngressActive(
		ctx context.Context,
		externalID string,
		leaseToken uuid.UUID,
		hostname string,
		publicEndpoint string,
	) error

	ScheduleGatewayIngressRetry(
		ctx context.Context,
		externalID string,
		leaseToken uuid.UUID,
		message string,
		nextRetryAt time.Time,
	) error

	MarkGatewayIngressFailed(
		ctx context.Context,
		externalID string,
		leaseToken uuid.UUID,
		message string,
	) error
	MarkGatewayDecommissioned(
		ctx context.Context,
		externalID string,
		leaseToken uuid.UUID,
	) error
}

type GatewayProvisioner struct {
	repo               GatewayProvisioningRepository
	loadBalancer       aws.DedicatedGatewayLoadBalancerManager
	containerMgr       container.ContainerManager
	workerID           string
	listenerARN        string
	vpcID              string
	instanceID         string
	baseDomain         string
	priorityMin        int
	priorityMax        int
	maxRetries         int
	healthPollInterval time.Duration
	now                func() time.Time
	logger             zerolog.Logger
}

func NewGatewayProvisioner(
	repo GatewayProvisioningRepository,
	loadBalancer aws.DedicatedGatewayLoadBalancerManager,
	containerMgr container.ContainerManager,
	workerID string,
	listenerARN string,
	vpcID string,
	instanceID string,
	baseDomain string,
	priorityMin int,
	priorityMax int,
	logger zerolog.Logger,
) *GatewayProvisioner {
	if workerID == "" {
		workerID = "gateway-worker-" + uuid.NewString()[:8]
	}

	return &GatewayProvisioner{
		repo:               repo,
		loadBalancer:       loadBalancer,
		containerMgr:       containerMgr,
		workerID:           workerID,
		listenerARN:        strings.TrimSpace(listenerARN),
		vpcID:              strings.TrimSpace(vpcID),
		instanceID:         strings.TrimSpace(instanceID),
		baseDomain:         strings.TrimSuffix(strings.TrimSpace(baseDomain), "."),
		priorityMin:        priorityMin,
		priorityMax:        priorityMax,
		maxRetries:         10,
		healthPollInterval: 15 * time.Second,
		now:                time.Now,
		logger:             logger,
	}
}

func (p *GatewayProvisioner) ProcessNextJob(
	ctx context.Context,
) (bool, error) {
	job, err := p.repo.ClaimNextGatewayIngressJob(
		ctx,
		p.workerID,
		5*time.Minute,
	)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}

	p.logger.Info().
		Str("gateway_id", job.ExternalID).
		Str("status", job.ProvisioningStatus).
		Int("host_port", job.HostPort).
		Msg("dedicated gateway provisioning job claimed")

	switch job.ProvisioningStatus {
	case gatewayStatusContainerReady:
		err = p.handleContainerReady(ctx, job)

	case gatewayStatusRegisteringTarget:
		err = p.handleRegisteringTarget(ctx, job)

	case gatewayStatusCreatingListenerRule:
		err = p.handleCreatingListenerRule(ctx, job)

	case gatewayStatusWaitingForTargetHealth:
		err = p.handleWaitingForTargetHealth(ctx, job)

	case gatewayStatusDeprovisioning:
		err = p.handleDeprovisioning(ctx, job)

	default:
		err = fmt.Errorf(
			"unsupported gateway provisioning status %q",
			job.ProvisioningStatus,
		)
	}

	if err != nil {
		p.logger.Error().
			Err(err).
			Str("gateway_id", job.ExternalID).
			Str("status", job.ProvisioningStatus).
			Msg("dedicated gateway provisioning step failed")
		return true, err
	}

	return true, nil
}

func (p *GatewayProvisioner) handleContainerReady(
	ctx context.Context,
	job *storage.GatewayProvisioningJob,
) error {
	if job.HostPort < 1 || job.HostPort > 65535 {
		return p.failJob(
			ctx,
			job,
			fmt.Sprintf("invalid Docker host port %d", job.HostPort),
		)
	}

	targetGroupARN, err := p.loadBalancer.CreateGatewayTargetGroup(
		ctx,
		gatewayTargetGroupName(job.ExternalID),
		p.vpcID,
	)
	if err != nil {
		return p.retryOrFail(ctx, job, err)
	}

	return p.repo.MarkGatewayTargetGroupCreated(
		ctx,
		job.ExternalID,
		job.LeaseToken,
		targetGroupARN,
	)
}

func (p *GatewayProvisioner) handleRegisteringTarget(
	ctx context.Context,
	job *storage.GatewayProvisioningJob,
) error {
	if job.TargetGroupARN == "" {
		return p.failJob(
			ctx,
			job,
			"target group ARN is missing",
		)
	}

	err := p.loadBalancer.RegisterGatewayTarget(
		ctx,
		job.TargetGroupARN,
		p.instanceID,
		int32(job.HostPort),
	)
	if err != nil {
		return p.retryOrFail(ctx, job, err)
	}

	return p.repo.MarkGatewayTargetRegistered(
		ctx,
		job.ExternalID,
		job.LeaseToken,
	)
}

func (p *GatewayProvisioner) handleCreatingListenerRule(
	ctx context.Context,
	job *storage.GatewayProvisioningJob,
) error {
	if job.TargetGroupARN == "" {
		return p.failJob(
			ctx,
			job,
			"target group ARN is missing before listener-rule creation",
		)
	}

	priority, err := p.repo.ReserveGatewayListenerRulePriority(
		ctx,
		job.ExternalID,
		job.LeaseToken,
		p.priorityMin,
		p.priorityMax,
	)
	if err != nil {
		return p.retryOrFail(ctx, job, err)
	}

	hostname := gatewayHostname(job.ExternalID, p.baseDomain)

	ruleARN, err := p.loadBalancer.CreateGatewayHostRule(
		ctx,
		p.listenerARN,
		hostname,
		job.TargetGroupARN,
		int32(priority),
	)
	if err != nil {
		return p.retryOrFail(ctx, job, err)
	}

	return p.repo.MarkGatewayListenerRuleCreated(
		ctx,
		job.ExternalID,
		job.LeaseToken,
		ruleARN,
		p.now().Add(p.healthPollInterval),
	)
}

func (p *GatewayProvisioner) handleWaitingForTargetHealth(
	ctx context.Context,
	job *storage.GatewayProvisioningJob,
) error {
	if job.TargetGroupARN == "" {
		return p.failJob(
			ctx,
			job,
			"target group ARN is missing during health check",
		)
	}

	state, err := p.loadBalancer.GetGatewayTargetHealth(
		ctx,
		job.TargetGroupARN,
		p.instanceID,
		int32(job.HostPort),
	)
	if err != nil {
		return p.retryOrFail(ctx, job, err)
	}

	switch strings.ToLower(strings.TrimSpace(state)) {
	case "healthy":
		hostname := gatewayHostname(job.ExternalID, p.baseDomain)

		return p.repo.MarkGatewayIngressActive(
			ctx,
			job.ExternalID,
			job.LeaseToken,
			hostname,
			"https://"+hostname,
		)

	case "unhealthy":
		return p.retryOrFail(
			ctx,
			job,
			errors.New("ALB reports the dedicated gateway target as unhealthy"),
		)

	default:
		return p.repo.RescheduleGatewayHealthCheck(
			ctx,
			job.ExternalID,
			job.LeaseToken,
			p.now().Add(p.healthPollInterval),
		)
	}
}

// handleDeprovisioning removes the public ALB resources before removing the
// Docker runtime and finally soft-deleting the gateway database row.
func (p *GatewayProvisioner) handleDeprovisioning(
	ctx context.Context,
	job *storage.GatewayProvisioningJob,
) error {
	if job.ListenerRuleARN != "" {
		if err := p.loadBalancer.DeleteGatewayHostRule(
			ctx,
			job.ListenerRuleARN,
		); err != nil {
			return p.retryOrFail(
				ctx,
				job,
				fmt.Errorf("delete gateway listener rule: %w", err),
			)
		}
	}

	if job.TargetGroupARN != "" && job.HostPort > 0 {
		if err := p.loadBalancer.DeregisterGatewayTarget(
			ctx,
			job.TargetGroupARN,
			p.instanceID,
			int32(job.HostPort),
		); err != nil {
			return p.retryOrFail(
				ctx,
				job,
				fmt.Errorf("deregister gateway target: %w", err),
			)
		}
	}

	if job.TargetGroupARN != "" {
		if err := p.loadBalancer.DeleteGatewayTargetGroup(
			ctx,
			job.TargetGroupARN,
		); err != nil {
			return p.retryOrFail(
				ctx,
				job,
				fmt.Errorf("delete gateway target group: %w", err),
			)
		}
	}

	if p.containerMgr == nil {
		return p.retryOrFail(
			ctx,
			job,
			errors.New("Docker container manager is not configured"),
		)
	}

	if err := p.containerMgr.Decommission(
		ctx,
		job.ExternalID,
	); err != nil {
		return p.retryOrFail(
			ctx,
			job,
			fmt.Errorf("remove gateway container: %w", err),
		)
	}

	return p.repo.MarkGatewayDecommissioned(
		ctx,
		job.ExternalID,
		job.LeaseToken,
	)
}

func (p *GatewayProvisioner) retryOrFail(
	ctx context.Context,
	job *storage.GatewayProvisioningJob,
	operationErr error,
) error {
	message := strings.TrimSpace(operationErr.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}

	nextAttempt := job.RetryCount + 1
	if nextAttempt >= p.maxRetries {
		return p.repo.MarkGatewayIngressFailed(
			ctx,
			job.ExternalID,
			job.LeaseToken,
			message,
		)
	}

	return p.repo.ScheduleGatewayIngressRetry(
		ctx,
		job.ExternalID,
		job.LeaseToken,
		message,
		p.now().Add(CalculateRetryDelay(nextAttempt)),
	)
}

func (p *GatewayProvisioner) failJob(
	ctx context.Context,
	job *storage.GatewayProvisioningJob,
	message string,
) error {
	return p.repo.MarkGatewayIngressFailed(
		ctx,
		job.ExternalID,
		job.LeaseToken,
		message,
	)
}

func gatewayTargetGroupName(externalID string) string {
	return "eg-gw-" + gatewayResourceSuffix(externalID)
}

func gatewayHostname(externalID string, baseDomain string) string {
	return "gw-" + gatewayResourceSuffix(externalID) + "." + baseDomain
}

func gatewayResourceSuffix(externalID string) string {
	value := strings.ToLower(strings.TrimSpace(externalID))
	value = strings.TrimPrefix(value, "gw_")
	value = strings.TrimPrefix(value, "gw-")

	var cleaned strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			cleaned.WriteRune(char)
		case char >= '0' && char <= '9':
			cleaned.WriteRune(char)
		case cleaned.Len() > 0:
			cleaned.WriteByte('-')
		}
	}

	result := strings.Trim(cleaned.String(), "-")
	if result != "" && len(result) <= 20 {
		return result
	}

	hash := sha256.Sum256([]byte(externalID))
	return hex.EncodeToString(hash[:])[:12]
}
