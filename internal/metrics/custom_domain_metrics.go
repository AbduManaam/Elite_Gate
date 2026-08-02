package metrics

import (
	"errors"
	"strings"

	acmTypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	elbv2Types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/prometheus/client_golang/prometheus"
)

// Bounded Enum Label Constants
const (
	OpProvision   = "provision"
	OpDeprovision = "deprovision"

	CategoryTransient = "transient"
	CategoryTerminal  = "terminal"

	ReasonAccessDenied         = "access_denied"
	ReasonInvalidParameter     = "invalid_parameter"
	ReasonResourceNotFound     = "resource_not_found"
	ReasonDefaultCertRejection = "default_cert_rejection"
	ReasonQuotaExceeded        = "quota_exceeded"
	ReasonUnknown              = "unknown"

	StepRequestCertificate  = "request_certificate"
	StepDescribeCertificate = "describe_certificate"
	StepAttachCertificate   = "attach_certificate"
	StepDetachCertificate   = "detach_certificate"
	StepDeleteCertificate   = "delete_certificate"
	StepCompleteJob         = "complete_job"
)

// SanitizeFailureReason checks ACM & ELBv2 typed errors first, with string fallback.
func SanitizeFailureReason(err error) string {
	if err == nil {
		return ReasonUnknown
	}

	// ACM Typed Errors
	var acmAccessDenied *acmTypes.AccessDeniedException
	var acmInvalidParam *acmTypes.InvalidParameterException
	var acmNotFound *acmTypes.ResourceNotFoundException

	if errors.As(err, &acmAccessDenied) {
		return ReasonAccessDenied
	}
	if errors.As(err, &acmInvalidParam) {
		return ReasonInvalidParameter
	}
	if errors.As(err, &acmNotFound) {
		return ReasonResourceNotFound
	}

	// ELBv2 Typed Errors
	var albListenerNotFound *elbv2Types.ListenerNotFoundException
	var albCertNotFound *elbv2Types.CertificateNotFoundException
	var albTooManyCerts *elbv2Types.TooManyCertificatesException
	var albTooManyListeners *elbv2Types.TooManyListenersException

	if errors.As(err, &albListenerNotFound) || errors.As(err, &albCertNotFound) {
		return ReasonResourceNotFound
	}
	if errors.As(err, &albTooManyCerts) || errors.As(err, &albTooManyListeners) {
		return ReasonQuotaExceeded
	}

	lowered := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lowered, "access") || strings.Contains(lowered, "denied"):
		return ReasonAccessDenied
	case strings.Contains(lowered, "invalid"):
		return ReasonInvalidParameter
	case strings.Contains(lowered, "not found"):
		return ReasonResourceNotFound
	case strings.Contains(lowered, "default"):
		return ReasonDefaultCertRejection
	case strings.Contains(lowered, "quota") || strings.Contains(lowered, "limit"):
		return ReasonQuotaExceeded
	default:
		return ReasonUnknown
	}
}

type CustomDomainMetrics struct {
	Registry                       *prometheus.Registry
	JobsClaimed                    *prometheus.CounterVec
	JobsCompleted                  *prometheus.CounterVec
	JobsFailed                     *prometheus.CounterVec
	JobsRetried                    *prometheus.CounterVec
	JobsActive                     *prometheus.GaugeVec
	WorkerHeartbeatTimestamp       prometheus.Gauge
	WorkerLastSuccessTimestamp     prometheus.Gauge
	WorkerLastFailureTimestamp     prometheus.Gauge
	ProvisionDurationSeconds       *prometheus.HistogramVec
	DeprovisionDurationSeconds     *prometheus.HistogramVec
	JobStepDurationSeconds         *prometheus.HistogramVec
	QueueDepth                     *prometheus.GaugeVec
	QueueDepthCollectionErrors     prometheus.Counter
	QueueDepthLastSuccessTimestamp prometheus.Gauge
	AWSAPICalls                    *prometheus.CounterVec
	AWSAPIDurationSeconds          *prometheus.HistogramVec
	AWSAPIFailures                 *prometheus.CounterVec
}

func NewCustomDomainMetrics(reg *prometheus.Registry) *CustomDomainMetrics {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}

	m := &CustomDomainMetrics{
		Registry: reg,
		JobsClaimed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "custom_domain_jobs_claimed_total",
				Help: "Total custom domain jobs claimed by workers",
			},
			[]string{"operation"},
		),
		JobsCompleted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "custom_domain_jobs_completed_total",
				Help: "Total custom domain jobs completed successfully",
			},
			[]string{"operation"},
		),
		JobsFailed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "custom_domain_jobs_failed_total",
				Help: "Total custom domain terminal job failures",
			},
			[]string{"operation", "failure_reason"},
		),
		JobsRetried: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "custom_domain_jobs_retried_total",
				Help: "Total custom domain transient retry attempts scheduled",
			},
			[]string{"operation", "error_category"},
		),
		JobsActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "custom_domain_jobs_active",
				Help: "Number of jobs currently being processed by worker",
			},
			[]string{"operation"},
		),
		WorkerHeartbeatTimestamp: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "custom_domain_worker_heartbeat_timestamp_seconds",
				Help: "Unix timestamp of last worker loop heartbeat",
			},
		),
		WorkerLastSuccessTimestamp: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "custom_domain_worker_last_success_timestamp_seconds",
				Help: "Unix timestamp of last successful job completion",
			},
		),
		WorkerLastFailureTimestamp: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "custom_domain_worker_last_failure_timestamp_seconds",
				Help: "Unix timestamp of last terminal job failure",
			},
		),
		ProvisionDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "custom_domain_provision_duration_seconds",
				Help:    "End-to-end custom domain provisioning duration",
				Buckets: []float64{5, 15, 30, 60, 120, 300, 600, 1200},
			},
			[]string{"result"},
		),
		DeprovisionDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "custom_domain_deprovision_duration_seconds",
				Help:    "End-to-end custom domain deprovisioning duration",
				Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
			},
			[]string{"result"},
		),
		JobStepDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "custom_domain_job_step_duration_seconds",
				Help:    "Individual worker job step execution duration",
				Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"step"},
		),
		QueueDepth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "custom_domain_queue_depth",
				Help: "Current queue backlog depth grouped by provisioning status",
			},
			[]string{"state"},
		),
		QueueDepthCollectionErrors: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "custom_domain_queue_depth_collection_errors_total",
				Help: "Total queue depth collection errors",
			},
		),
		QueueDepthLastSuccessTimestamp: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "custom_domain_queue_depth_last_success_timestamp_seconds",
				Help: "Unix timestamp of last successful queue depth collection",
			},
		),
		AWSAPICalls: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aws_api_calls_total",
				Help: "Total AWS API calls executed",
			},
			[]string{"service", "operation", "result"},
		),
		AWSAPIDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "aws_api_duration_seconds",
				Help:    "AWS API execution duration in seconds",
				Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"service", "operation", "result"},
		),
		AWSAPIFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "aws_api_failures_total",
				Help: "Total AWS API call failures",
			},
			[]string{"service", "operation", "error_category"},
		),
	}

	reg.MustRegister(
		m.JobsClaimed,
		m.JobsCompleted,
		m.JobsFailed,
		m.JobsRetried,
		m.JobsActive,
		m.WorkerHeartbeatTimestamp,
		m.WorkerLastSuccessTimestamp,
		m.WorkerLastFailureTimestamp,
		m.ProvisionDurationSeconds,
		m.DeprovisionDurationSeconds,
		m.JobStepDurationSeconds,
		m.QueueDepth,
		m.QueueDepthCollectionErrors,
		m.QueueDepthLastSuccessTimestamp,
		m.AWSAPICalls,
		m.AWSAPIDurationSeconds,
		m.AWSAPIFailures,
	)

	return m
}
