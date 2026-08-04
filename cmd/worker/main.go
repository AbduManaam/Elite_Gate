package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"elitegate/internal/aws"
	"elitegate/internal/config"
	"elitegate/internal/container"
	"elitegate/internal/domain"
	"elitegate/internal/metrics"
	"elitegate/internal/storage"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().Timestamp().Str("component", "worker").Logger()

	cfg, err := config.LoadConfigForService(config.ServiceWorker)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}
	if cfg.Database.DSN == "" {
		logger.Fatal().Msg("database connection DSN (POSTGRES_DSN) is required")
	}

	db, err := storage.NewPostgres(logger, cfg.Database)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	containerMgr, err := container.NewDockerContainerManager(
		cfg.Server.AdminAPIURL,
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Auth.JWTSecret,
		cfg.Server.GatewayImageName,
		cfg.Server.GatewayHostPublic,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init container manager")
	}
	defer containerMgr.Close()

	gatewayRepo := storage.NewGatewayRepo(db)

	staleAfter, err := time.ParseDuration(cfg.Server.DrainStaleAfter)
	if err != nil {
		logger.Fatal().Err(err).Msg("invalid server.drain_stale_after")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize Prometheus metrics registry & custom domain metrics
	reg := prometheus.NewRegistry()
	custMetrics := metrics.NewCustomDomainMetrics(reg)

	// Start Internal Metrics & Health HTTP Server
	metricsServer := StartWorkerMetricsServer(reg, logger)
	defer func() {
		_ = metricsServer.Shutdown(context.Background())
	}()

	awsAutomationEnabled := os.Getenv("CUSTOM_DOMAIN_AWS_AUTOMATION_ENABLED") == "true"
	var awsClient *aws.Client
	if awsAutomationEnabled {
		region := os.Getenv("AWS_REGION")
		listenerARN := os.Getenv("ALB_HTTPS_LISTENER_ARN")

		client, err := aws.NewClient(ctx, region, listenerARN, true)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to initialize AWS client")
		}
		awsClient = client

		customDomainRepo := storage.NewCustomDomainRepo(db, logger)
		provisioner := NewProvisioner(customDomainRepo, awsClient, awsClient, "worker-main", listenerARN, logger).
			WithMetrics(custMetrics)

		// Queue depth polling loop (runs only when automation enabled)
		knownStates := []string{
			domain.ProvisioningStatusRequestingCertificate,
			domain.ProvisioningStatusWaitingForValidationRecord,
			domain.ProvisioningStatusWaitingForDNS,
			domain.ProvisioningStatusWaitingForCertificate,
			domain.ProvisioningStatusAttachingCertificate,
			domain.ProvisioningStatusDeprovisioning,
		}

		go func() {
			queueTicker := time.NewTicker(30 * time.Second)
			defer queueTicker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-queueTicker.C:
					// Reset all known states to zero before applying query results
					for _, st := range knownStates {
						custMetrics.QueueDepth.WithLabelValues(st).Set(0)
					}

					depths, qErr := customDomainRepo.GetProvisioningQueueDepth(ctx)
					if qErr != nil {
						custMetrics.QueueDepthCollectionErrors.Inc()
						logger.Error().Err(qErr).Msg("failed to collect queue depth metrics")
					} else {
						for st, cnt := range depths {
							custMetrics.QueueDepth.WithLabelValues(st).Set(float64(cnt))
						}
						custMetrics.QueueDepthLastSuccessTimestamp.Set(float64(time.Now().Unix()))
					}
				}
			}
		}()

		// Worker loop
		go func() {
			provTicker := time.NewTicker(5 * time.Second)
			defer provTicker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-provTicker.C:
					custMetrics.WorkerHeartbeatTimestamp.Set(float64(time.Now().Unix()))
					_, _ = provisioner.ProcessNextJob(ctx)
				}
			}
		}()
		logger.Info().Msg("ACM custom domain provisioner loop started")
	} else {
		logger.Info().Msg("CUSTOM_DOMAIN_AWS_AUTOMATION_ENABLED is false; ACM provisioner loop disabled")
	}

	dedicatedGatewayAWSConfig, err := loadDedicatedGatewayAutomationConfig()
	if err != nil {
		logger.Fatal().
			Err(err).
			Msg("invalid dedicated gateway AWS automation configuration")
	}

	if dedicatedGatewayAWSConfig.Enabled {
		var gatewayAWSClient aws.DedicatedGatewayLoadBalancerManager
		if awsClient != nil && os.Getenv("AWS_REGION") == dedicatedGatewayAWSConfig.Region && os.Getenv("ALB_HTTPS_LISTENER_ARN") == dedicatedGatewayAWSConfig.ListenerARN {
			gatewayAWSClient = awsClient
		} else {
			client, err := aws.NewClient(
				ctx,
				dedicatedGatewayAWSConfig.Region,
				dedicatedGatewayAWSConfig.ListenerARN,
				true,
			)
			if err != nil {
				logger.Fatal().
					Err(err).
					Msg("failed to initialize dedicated gateway AWS client")
			}
			gatewayAWSClient = client
		}

		gatewayProvisioner := NewGatewayProvisioner(
			gatewayRepo,
			gatewayAWSClient,
			"gateway-worker-main",
			dedicatedGatewayAWSConfig.ListenerARN,
			dedicatedGatewayAWSConfig.VPCID,
			dedicatedGatewayAWSConfig.InstanceID,
			dedicatedGatewayAWSConfig.BaseDomain,
			dedicatedGatewayAWSConfig.PriorityMin,
			dedicatedGatewayAWSConfig.PriorityMax,
			logger,
		)

		go func() {
			gatewayTicker := time.NewTicker(5 * time.Second)
			defer gatewayTicker.Stop()

			for {
				select {
				case <-ctx.Done():
					return

				case <-gatewayTicker.C:
					processed, processErr := gatewayProvisioner.ProcessNextJob(ctx)
					if processErr != nil {
						logger.Error().
							Err(processErr).
							Msg("dedicated gateway provisioning iteration failed")
						continue
					}

					if processed {
						logger.Debug().
							Msg("dedicated gateway provisioning job processed")
					}
				}
			}
		}()

		logger.Info().
			Str("base_domain", dedicatedGatewayAWSConfig.BaseDomain).
			Int("priority_min", dedicatedGatewayAWSConfig.PriorityMin).
			Int("priority_max", dedicatedGatewayAWSConfig.PriorityMax).
			Msg("dedicated gateway ALB provisioner loop started")
	} else {
		logger.Info().
			Msg("dedicated gateway AWS automation is disabled")
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	logger.Info().Dur("stale_after", staleAfter).Msg("worker started: reconciling stale draining gateways")

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("worker shutting down")
			return
		case <-ticker.C:
			reconcileStaleDraining(ctx, gatewayRepo, containerMgr, staleAfter, logger)
		}
	}
}

// reconcileStaleDraining finishes decommissioning any gateway that has
// been stuck in "draining" longer than staleAfter.
func reconcileStaleDraining(
	ctx context.Context,
	gatewayRepo *storage.GatewayRepo,
	containerMgr container.ContainerManager,
	staleAfter time.Duration,
	logger zerolog.Logger,
) {
	stale, err := gatewayRepo.ListStaleDraining(ctx, staleAfter)
	if err != nil {
		logger.Error().Err(err).Msg("failed to list stale draining gateways")
		return
	}
	if len(stale) == 0 {
		return
	}

	logger.Info().Int("count", len(stale)).Msg("reconciling stale draining gateways")

	for _, g := range stale {
		log := logger.With().Str("external_id", g.ExternalID).
			Time("drain_started_at", g.DrainStartedAt).Logger()

		log.Warn().Msg("gateway stuck draining past stale threshold; finishing decommission")

		if err := containerMgr.Decommission(ctx, g.ExternalID); err != nil {
			log.Error().Err(err).Msg("reconciler: failed to stop container runtime")
			continue
		}
		if err := gatewayRepo.Decommission(ctx, g.ExternalID); err != nil {
			log.Error().Err(err).Msg("reconciler: failed to finalize DB row")
			continue
		}

		log.Info().Msg("reconciler: gateway decommissioned successfully")
	}
}
