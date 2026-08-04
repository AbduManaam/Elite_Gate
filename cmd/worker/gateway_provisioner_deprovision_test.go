package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"elitegate/internal/storage"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type fakeGatewayProvisioningRepo struct {
	job                   *storage.GatewayProvisioningJob
	jobClaimed            bool
	operations            *[]string
	retryScheduled        bool
	markedDecommissioned  bool
	scheduleRetryErr      error
	markDecommissionedErr error
}

func (r *fakeGatewayProvisioningRepo) ClaimNextGatewayIngressJob(
	_ context.Context,
	_ string,
	_ time.Duration,
) (*storage.GatewayProvisioningJob, error) {
	if r.jobClaimed {
		return nil, nil
	}

	r.jobClaimed = true
	return r.job, nil
}

func (r *fakeGatewayProvisioningRepo) MarkGatewayTargetGroupCreated(
	context.Context,
	string,
	uuid.UUID,
	string,
) error {
	return nil
}

func (r *fakeGatewayProvisioningRepo) MarkGatewayTargetRegistered(
	context.Context,
	string,
	uuid.UUID,
) error {
	return nil
}

func (r *fakeGatewayProvisioningRepo) ReserveGatewayListenerRulePriority(
	context.Context,
	string,
	uuid.UUID,
	int,
	int,
) (int, error) {
	return 1000, nil
}

func (r *fakeGatewayProvisioningRepo) MarkGatewayListenerRuleCreated(
	context.Context,
	string,
	uuid.UUID,
	string,
	time.Time,
) error {
	return nil
}

func (r *fakeGatewayProvisioningRepo) RescheduleGatewayHealthCheck(
	context.Context,
	string,
	uuid.UUID,
	time.Time,
) error {
	return nil
}

func (r *fakeGatewayProvisioningRepo) MarkGatewayIngressActive(
	context.Context,
	string,
	uuid.UUID,
	string,
	string,
) error {
	return nil
}

func (r *fakeGatewayProvisioningRepo) ScheduleGatewayIngressRetry(
	_ context.Context,
	_ string,
	_ uuid.UUID,
	_ string,
	_ time.Time,
) error {
	r.retryScheduled = true

	if r.operations != nil {
		*r.operations = append(*r.operations, "schedule_retry")
	}

	return r.scheduleRetryErr
}

func (r *fakeGatewayProvisioningRepo) MarkGatewayIngressFailed(
	_ context.Context,
	_ string,
	_ uuid.UUID,
	_ string,
) error {
	if r.operations != nil {
		*r.operations = append(*r.operations, "mark_failed")
	}

	return nil
}

func (r *fakeGatewayProvisioningRepo) MarkGatewayDecommissioned(
	_ context.Context,
	_ string,
	_ uuid.UUID,
) error {
	r.markedDecommissioned = true

	if r.operations != nil {
		*r.operations = append(*r.operations, "mark_decommissioned")
	}

	return r.markDecommissionedErr
}

type fakeGatewayLoadBalancer struct {
	operations           *[]string
	deleteRuleErr        error
	deregisterTargetErr  error
	deleteTargetGroupErr error
}

func (f *fakeGatewayLoadBalancer) CreateGatewayTargetGroup(
	context.Context,
	string,
	string,
) (string, error) {
	return "", nil
}

func (f *fakeGatewayLoadBalancer) RegisterGatewayTarget(
	context.Context,
	string,
	string,
	int32,
) error {
	return nil
}

func (f *fakeGatewayLoadBalancer) GetGatewayTargetHealth(
	context.Context,
	string,
	string,
	int32,
) (string, error) {
	return "healthy", nil
}

func (f *fakeGatewayLoadBalancer) CreateGatewayHostRule(
	context.Context,
	string,
	string,
	string,
	int32,
) (string, error) {
	return "", nil
}

func (f *fakeGatewayLoadBalancer) DeleteGatewayHostRule(
	context.Context,
	string,
) error {
	*f.operations = append(*f.operations, "delete_listener_rule")
	return f.deleteRuleErr
}

func (f *fakeGatewayLoadBalancer) DeregisterGatewayTarget(
	context.Context,
	string,
	string,
	int32,
) error {
	*f.operations = append(*f.operations, "deregister_target")
	return f.deregisterTargetErr
}

func (f *fakeGatewayLoadBalancer) DeleteGatewayTargetGroup(
	context.Context,
	string,
) error {
	*f.operations = append(*f.operations, "delete_target_group")
	return f.deleteTargetGroupErr
}

type fakeGatewayContainerManager struct {
	operations      *[]string
	decommissionErr error
}

func (f *fakeGatewayContainerManager) Provision(
	context.Context,
	string,
	string,
	string,
) (string, string, string, string, error) {
	return "", "", "", "", nil
}

func (f *fakeGatewayContainerManager) Decommission(
	_ context.Context,
	_ string,
) error {
	*f.operations = append(*f.operations, "remove_container")
	return f.decommissionErr
}

func TestGatewayProvisionerDeprovisioningOrder(t *testing.T) {
	t.Parallel()

	operations := []string{}

	job := &storage.GatewayProvisioningJob{
		ExternalID:         "gw_test123",
		HostPort:           32778,
		TargetGroupARN:     "target-group-arn",
		ListenerRuleARN:    "listener-rule-arn",
		ProvisioningStatus: gatewayStatusDeprovisioning,
		LeaseToken:         uuid.New(),
	}

	repo := &fakeGatewayProvisioningRepo{
		job:        job,
		operations: &operations,
	}

	loadBalancer := &fakeGatewayLoadBalancer{
		operations: &operations,
	}

	containerManager := &fakeGatewayContainerManager{
		operations: &operations,
	}

	provisioner := &GatewayProvisioner{
		repo:         repo,
		loadBalancer: loadBalancer,
		containerMgr: containerManager,
		workerID:     "test-worker",
		instanceID:   "i-test123",
		maxRetries:   10,
		now: func() time.Time {
			return time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC)
		},
		logger: zerolog.Nop(),
	}

	processed, err := provisioner.ProcessNextJob(context.Background())
	if err != nil {
		t.Fatalf("ProcessNextJob returned error: %v", err)
	}

	if !processed {
		t.Fatal("expected a deprovisioning job to be processed")
	}

	expected := []string{
		"delete_listener_rule",
		"deregister_target",
		"delete_target_group",
		"remove_container",
		"mark_decommissioned",
	}

	if !reflect.DeepEqual(operations, expected) {
		t.Fatalf(
			"unexpected cleanup order\nexpected: %v\nactual:   %v",
			expected,
			operations,
		)
	}

	if !repo.markedDecommissioned {
		t.Fatal("expected gateway database row to be finalized")
	}
}

func TestGatewayProvisionerRetriesBeforeRemovingContainer(t *testing.T) {
	t.Parallel()

	operations := []string{}

	job := &storage.GatewayProvisioningJob{
		ExternalID:         "gw_test456",
		HostPort:           32779,
		TargetGroupARN:     "target-group-arn",
		ListenerRuleARN:    "listener-rule-arn",
		ProvisioningStatus: gatewayStatusDeprovisioning,
		RetryCount:         0,
		LeaseToken:         uuid.New(),
	}

	repo := &fakeGatewayProvisioningRepo{
		job:        job,
		operations: &operations,
	}

	loadBalancer := &fakeGatewayLoadBalancer{
		operations:           &operations,
		deleteTargetGroupErr: errors.New("AWS dependency violation"),
	}

	containerManager := &fakeGatewayContainerManager{
		operations: &operations,
	}

	provisioner := &GatewayProvisioner{
		repo:         repo,
		loadBalancer: loadBalancer,
		containerMgr: containerManager,
		workerID:     "test-worker",
		instanceID:   "i-test123",
		maxRetries:   10,
		now: func() time.Time {
			return time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC)
		},
		logger: zerolog.Nop(),
	}

	processed, err := provisioner.ProcessNextJob(context.Background())
	if err != nil {
		t.Fatalf("ProcessNextJob returned error: %v", err)
	}

	if !processed {
		t.Fatal("expected a deprovisioning job to be processed")
	}

	expected := []string{
		"delete_listener_rule",
		"deregister_target",
		"delete_target_group",
		"schedule_retry",
	}

	if !reflect.DeepEqual(operations, expected) {
		t.Fatalf(
			"unexpected operations after AWS failure\nexpected: %v\nactual:   %v",
			expected,
			operations,
		)
	}

	if !repo.retryScheduled {
		t.Fatal("expected deprovisioning retry to be scheduled")
	}

	if repo.markedDecommissioned {
		t.Fatal("gateway must not be finalized while AWS cleanup is incomplete")
	}

	for _, operation := range operations {
		if operation == "remove_container" {
			t.Fatal("container must remain running when AWS cleanup fails")
		}
	}
}
