EliteGate Backend

Multi-tenant API gateway control plane and data plane built with Go, PostgreSQL, Redis, Docker, Prometheus, and AWS integrations.

EliteGate provides a centrally managed gateway layer in front of backend services. Teams can create isolated projects, register upstream services and targets, define routes and traffic policies, issue API keys, provision project gateways, monitor traffic, and manage custom domains without embedding gateway logic into every service.

Project status: EliteGate contains substantial working functionality, but some production-hardening and AWS public-endpoint automation remain incomplete. Review Current Status & Limitations before treating it as production-ready.

Table of Contents

Why EliteGate

What Is Implemented

Architecture

How a Customer Uses EliteGate

Request Flow

Technology Stack

Repository Structure

Quick Start for Developers

Configuration

Running the Services

Testing

Docker

Authentication & Authorization

Multi-Tenancy

Gateway Synchronization

Deployment

Observability

Current Status & Limitations

Troubleshooting

Documentation

Contributing

License

Why EliteGate

Applications with several backend services often repeat the same infrastructure concerns in every service:

authentication and authorization;

API-key validation;

rate limiting;

CORS;

IP allow/block rules;

routing;

health checking;

load balancing;

metrics and operational visibility.

EliteGate moves those concerns into a managed gateway layer.

Client
  ↓
EliteGate Gateway
  ↓
Route + Policy Evaluation
  ↓
Target Selection
  ↓
Customer Backend Service

A separate Admin control plane stores the configuration, while project-specific gateway runtimes enforce it on live traffic.

What Is Implemented

Capability

Status

Admin control plane

✅ Implemented

HTTP reverse-proxy data plane

✅ Implemented

JWT access + refresh authentication

✅ Implemented

Google OAuth with PKCE

✅ Implemented when configured

Password reset

✅ Implemented when SMTP is configured

Project-scoped routes, upstreams, targets and policies

✅ Implemented

API-key creation, rotation and revocation

✅ Implemented

Redis API-key cache

✅ Implemented

Redis distributed rate limiting with memory fallback

✅ Implemented

Weighted round robin / least connections

✅ Implemented

Active upstream health checks

✅ Implemented

Pull-based gateway configuration sync

✅ Implemented

Docker gateway provisioning

✅ Implemented

Custom-domain ownership verification

✅ Implemented

Terraform AWS foundation

✅ Implemented

Architecture

EliteGate separates management from request processing.

flowchart LR
    User[Administrator] --> UI[EliteGate Admin UI]
    UI -->|HTTPS + JWT| Admin[Go Admin Control Plane]

    Admin --> PG[(PostgreSQL)]
    Admin --> Docker[Docker Engine]
    Admin --> Prom[Prometheus]

    Worker[Background Worker] --> PG
    Worker --> Docker
    Worker --> ACM[AWS ACM]
    Worker --> ALB[AWS ALB]

    Gateway[Project Gateway] -->|Project Sync Token| Admin
    Gateway --> Redis[(Redis)]

    Client[Customer Client] --> Gateway
    Gateway --> ServiceA[Backend Target A]
    Gateway --> ServiceB[Backend Target B]

Main processes

Process

Responsibility

cmd/admin

Accounts, projects, RBAC, CRUD, gateway provisioning, audit logs, metrics and sync snapshots

cmd/gateway

Route matching, policy enforcement, rate limiting, load balancing and proxying

cmd/worker

Gateway lifecycle reconciliation and custom-domain AWS provisioning work

cmd/token

Development/test client JWT generation

cmd/testbackend

Small local upstream used for proxy testing

PostgreSQL is the control-plane source of truth. Gateways do not use PostgreSQL as their runtime configuration store; they pull a project snapshot from the Admin API and keep the active configuration in memory.

How a Customer Uses EliteGate

This is the normal product workflow from the Admin Console.

1. Sign up or sign in

Create an account using company name, username, email and password, or sign in with an existing account.

Signup creates:

an administrator account;

an initial project;

Owner membership for that project.

Google OAuth is also available when configured.

2. Create or select a project

A project is the tenant boundary in EliteGate.

Use separate projects when you need separate routes, upstreams, API keys, policies, gateways and audit history.

Typical examples:

YUMZY Production
YUMZY Staging
Payments Platform
Internal APIs

3. Invite team members

Project Owners can add registered users and assign:

owner

editor

viewer

The backend remains the authoritative permission boundary; UI visibility alone is not treated as security.

4. Register an upstream service

An upstream represents the backend service that EliteGate will call.

Example:

Name: orders-service
Protocol: HTTP
Target URL: http://orders-service:8080
Health Path: /healthz
Strategy: round_robin

The upstream must be reachable from the gateway runtime, not merely from the user's browser.

Avoid using localhost for a remote/containerized backend unless the target is actually running inside the same runtime namespace.

5. Add targets

An upstream can contain one or more targets.

Example:

orders-service
├── http://10.0.2.15:8080
├── http://10.0.2.16:8080
└── http://10.0.2.17:8080

Targets can include weights and enabled/disabled state.

6. Choose a load-balancing strategy

Current supported strategies are:

Round Robin

Least Connections

The selected strategy is included in the gateway's next configuration snapshot.

7. Create a route

A route connects a public gateway path to an upstream.

Example:

Gateway route: /api/orders
Upstream:      orders-service
Match:         prefix
Methods:       GET, POST

After the configuration is loaded by the gateway, matching requests are sent to the selected upstream target.

Current HTTP proxy behavior removes the leading /api segment before forwarding. For example, /api/orders is forwarded as /orders.

8. Add security and traffic policies

Policies can configure:

authentication requirement;

requests-per-minute rate limits;

allowed origins;

roles;

scopes;

IP allowlists;

IP blocklists.

Example browser origin:

https://app.customer.com

For local development:

http://localhost:5173

9. Create an API key when required

API keys can contain:

name;

expiry;

roles;

scopes.

The raw key is displayed only when created or rotated. Copy it immediately.

Do not embed privileged API keys in public frontend JavaScript.

10. Provision a gateway

From the project gateway section, provision a project gateway.

The backend:

creates a gateway record;

creates a Docker container;

injects project identity/configuration;

allocates a host port;

waits for health;

returns gateway metadata.

Current generated endpoint format is:

http://<public_host>:<public_port>

Automatic per-gateway ALB target-group, listener-rule, DNS and HTTPS creation is not complete in the current source.

11. Test traffic through EliteGate

With an API key:

curl -i \
  -H "X-API-Key: <generated-key>" \
  http://<gateway-host>:<gateway-port>/api/orders

With a JWT:

curl -i \
  -H "Authorization: Bearer <customer-jwt>" \
  http://<gateway-host>:<gateway-port>/api/orders

Common gateway responses:

Status

Meaning

401

Missing or invalid authentication

403

Policy, role, scope or IP denial

404

No matching route

429

Rate limit exceeded

502

Proxy/target failure

503

No usable target or dependency unavailable

12. Point the customer application at EliteGate

Before:

const api = axios.create({
  baseURL: 'https://orders-api.customer.com',
});

After:

const api = axios.create({
  baseURL: 'https://gateway.customer.com',
});

During the current direct host-port deployment:

const api = axios.create({
  baseURL: 'http://<gateway-host>:<gateway-port>',
});

Production applications should use HTTPS once their ALB/custom-domain routing is fully configured.

13. Monitor and operate

Use the Admin Console for:

project resource summaries;

traffic/system analytics;

audit logs;

gateway status;

gateway reloads;

custom-domain provisioning state.

Request Flow

For HTTP traffic:

Client request
  ↓
Route matcher
  ↓
Metrics
  ↓
IP policy
  ↓
CORS policy
  ↓
JWT / API-key authentication
  ↓
Rate limiter
  ↓
Load balancer
  ↓
Reverse proxy
  ↓
Customer backend

The gateway can reject a request before proxying when a route, security policy, quota or healthy target requirement is not satisfied.

Technology Stack

Area

Technology

Language

Go 1.25.4

Admin HTTP API

Gin

Gateway HTTP runtime

Go net/http / reverse proxy

Database

PostgreSQL

Cache / distributed limits

Redis

Authentication

JWT, refresh tokens, Google OAuth 2.0 + PKCE

gRPC

google.golang.org/grpc

Containers

Docker Engine SDK

Metrics

Prometheus

Logging

Zerolog

Configuration

Viper + environment variables

Migrations

golang-migrate

AWS

ACM, ELBv2, EC2, RDS, ElastiCache, ECR, SSM, Secrets Manager

Infrastructure as Code

Terraform

CI/CD

GitHub Actions

Repository Structure

.
├── .github/workflows/
│   ├── ci.yml
│   └── cd.yml
├── cmd/
│   ├── admin/
│   ├── gateway/
│   ├── worker/
│   ├── token/
│   └── testbackend/
├── deploy/
│   ├── docker-compose.yml
│   ├── docker/
│   └── scripts/
├── infra/terraform/
├── internal/
│   ├── admin/
│   ├── auth/
│   ├── aws/
│   ├── config/
│   ├── container/
│   ├── gateway/
│   ├── ratelimit/
│   └── storage/
├── migrations/
├── tests/
├── Taskfile.yml
├── go.mod
└── go.sum

Quick Start for Developers

Prerequisites

Install:

Go 1.25.4 or the version declared by go.mod;

Docker Engine;

Docker Compose;

PostgreSQL 16 and Redis 7, or use the included Compose services.

1. Clone and enter the repository

git clone <repository-url>
cd <backend-repository>

2. Create the Docker network

The Compose configuration expects an external network:

docker network create elitegate_net

You only need to create it once.

3. Start PostgreSQL and Redis

docker compose -f deploy/docker-compose.yml up -d postgres redis

4. Create .env

Create a root .env using safe development values.

APP_ENV=development

ADMIN_PORT=:9090
GATEWAY_PORT=:8080
GRPC_GATEWAY_PORT=:50051
ADMIN_API_URL=http://localhost:9090

POSTGRES_DSN=postgres://postgres:change-me@localhost:5433/elitegate_db?sslmode=disable

REDIS_ADDR=localhost:6379
REDIS_PASSWORD=change-me

JWT_SECRET=replace-with-at-least-32-random-bytes
GATEWAY_IMAGE_NAME=elitegate-gateway:latest
GATEWAY_PUBLIC_HOST=localhost

ALLOWED_ORIGINS=http://localhost:5173

FRONTEND_URL=http://localhost:5173
GOOGLE_REDIRECT_URL=http://localhost:9090/admin/google/callback

SMTP_ENABLED=false

CUSTOM_DOMAIN_AWS_AUTOMATION_ENABLED=false
AWS_REGION=ap-south-1

Generate strong development secrets when required:

openssl rand -base64 48

Do not commit real secrets.

5. Start the Admin API

go run ./cmd/admin

The Admin startup connects to PostgreSQL and applies migrations.

Verify:

curl http://localhost:9090/healthz

6. Create an account/project

The easiest method is to run the frontend and use Signup.

For API-only testing:

curl -X POST http://localhost:9090/admin/signup \
  -H 'Content-Type: application/json' \
  -d '{
    "username":"local-admin",
    "email":"local-admin@example.com",
    "password":"ChangeMe123!",
    "company":"Local Example",
    "plan":"free"
  }'

Save the returned:

access token;

project ID.

7. Configure a project

Use the Admin Console or Admin API to create:

Project
  ↓
Upstream
  ↓
Target(s)
  ↓
Policy
  ↓
Route

For local proxy testing, start the included test backend:

go run ./cmd/testbackend

8. Run the gateway

Set a valid project ID before starting the gateway:

export PROJECT_ID='<project-uuid>'

Then:

go run ./cmd/gateway

Verify the gateway health endpoint configured by the runtime, then test a configured route, for example:

curl http://localhost:8080/api/orders

The gateway requires a successful initial project sync; it will not intentionally start with an empty routing configuration.

9. Run the worker

go run ./cmd/worker

The worker requires PostgreSQL. Docker lifecycle reconciliation requires Docker Engine access. AWS custom-domain operations require the related AWS configuration and IAM permissions.

Configuration

Important variables:

Variable

Required

Purpose

JWT_SECRET

Yes

Admin JWT signing and project-derived secret root

POSTGRES_DSN

Yes

Control-plane PostgreSQL connection

PROJECT_ID

Gateway: Yes

Gateway tenant identity

GATEWAY_IMAGE_NAME

Provisioning: Yes

Image used for dynamic gateways

GATEWAY_PUBLIC_HOST

Admin startup: Yes

Host associated with generated gateway ports

ALLOWED_ORIGINS

Yes

Admin API CORS allowlist

REDIS_ADDR

No

Redis endpoint

REDIS_PASSWORD

No

Redis password

ADMIN_API_URL

Gateway

Control-plane URL

ROUTE_RELOAD_INTERVAL

No

Snapshot polling interval

GOOGLE_CLIENT_ID

OAuth only

Google OAuth client

GOOGLE_CLIENT_SECRET

OAuth only

Google OAuth secret

GOOGLE_REDIRECT_URL

OAuth only

Exact backend OAuth callback

SMTP_ENABLED

Feature dependent

Password-reset mail

CUSTOM_DOMAIN_AWS_AUTOMATION_ENABLED

No

Enables AWS custom-domain worker

AWS_REGION

AWS automation

AWS SDK region

ALB_HTTPS_LISTENER_ARN

AWS automation

Existing HTTPS listener

See internal/config/config.go and the repository configuration files for the complete reference.

Running the Services

Typical local ports:

Service

Port

Admin API

9090

Gateway HTTP

8080

Gateway gRPC

50051

PostgreSQL

5433

Redis

6379

Prometheus

9091

Grafana

3001

Admin-provisioned gateways are created dynamically and receive host ports at runtime.

Testing

Run the full Go test suite:

go test ./...

Run with the race detector:

go test ./... -race -count=1

Useful targeted commands:

go test ./internal/admin/service -count=1
go test ./internal/storage -count=1
go test ./internal/gateway/... -count=1
go test ./tests/... -count=1
go vet ./...

Formatting:

gofmt -w .

Integration tests requiring PostgreSQL use dedicated test DSNs; do not reuse production credentials.

Docker

The repository includes multi-stage Dockerfiles for Admin, Gateway and Worker.

Important Compose services include:

PostgreSQL

Redis

Admin

Gateway

Worker

Prometheus

Loki

Promtail

Grafana

Run the complete stack only after creating elitegate_net and supplying a valid gateway project configuration.

Dynamic project gateways are not static Compose services. The Admin API creates them through the Docker SDK.

Authentication & Authorization

EliteGate supports:

username/password login;

short-lived JWT access tokens;

opaque refresh-token rotation;

HttpOnly refresh cookies;

logout revocation;

Google OAuth Authorization Code flow with PKCE;

password-reset tokens stored as hashes;

project roles: Owner, Editor and Viewer;

separate super-admin capabilities.

Typical role model:

Role

Read

Configure resources

Manage membership/destructive owner operations

Viewer

✅

❌

❌

Editor

✅

✅

❌

Owner

✅

✅

✅

Multi-Tenancy

The project is the main tenant boundary.

Tenant protection is implemented through multiple layers including:

project ID in tenant API paths;

authenticated user identity;

membership validation;

RBAC;

tenant-aware database transactions;

project-scoped repository queries;

PostgreSQL row-level security;

project-bound gateway identity;

project-only configuration snapshots.

New code should always verify project ownership/membership and filter tenant-owned records by the active project.

Gateway Synchronization

Gateways fetch configuration through the internal project sync endpoint:

GET /internal/v1/projects/:project_id/sync
X-Gateway-Token: <project-derived-token>

Snapshot data includes the project-specific routing/configuration needed by the data plane.

Behavior:

failed initial sync → gateway startup fails;

later sync failure → last good in-memory configuration stays active;

Redis failure → supported memory fallback remains available;

successful sync → runtime structures are rebuilt and swapped into active configuration.

Deployment

The repository includes:

Terraform for AWS infrastructure;

GitHub Actions CI/CD;

ECR image publishing;

GitHub OIDC AWS authentication;

SSM-based EC2 deployment;

RDS PostgreSQL;

ElastiCache Redis;

Application Load Balancer;

Secrets Manager / Parameter Store integration.

High-level flow:

flowchart LR
    Push[Push to main] --> CI[GitHub Actions CI]
    CI --> ECR[Build + Push Images to ECR]
    ECR --> SSM[AWS SSM Deployment]
    SSM --> EC2[EC2 Docker Host]
    EC2 --> Health[Health Checks]
    Health -->|Success| Keep[Keep New Containers]
    Health -->|Failure| Rollback[Restore Previous Containers]

No long-lived AWS keys are required in GitHub when OIDC is configured correctly.

Observability

EliteGate exposes/uses:

health endpoints;

Prometheus request metrics;

upstream health metrics;

project metrics queries;

structured Zerolog logging;

audit logs;

optional Grafana/Loki/Promtail local stack.

Current Status & Limitations

This repository should not be described as fully production-ready yet.

Important current limitations include:

automatic per-dedicated-gateway ALB target group, dynamic instance-port registration, host-based listener rule, DNS record and public HTTPS endpoint creation is incomplete;

corresponding AWS cleanup during gateway decommission is incomplete;

gRPC does not yet have complete HTTP policy/load-balancing parity;

some tenant authorization and project-scoping paths require hardening;

project member role-change/removal has a backend route-parameter mismatch;

HTTP method configuration is stored but the current matcher does not fully enforce method restrictions;

API-key cache invalidation and Redis tenant namespacing need additional hardening;

upstream URL validation should be strengthened against unsafe network destinations;

Worker ECR/IaC alignment needs correction.

For a public deployment, resolve security and tenant-isolation findings before exposing the platform to untrusted users.

Troubleshooting

Admin cannot connect to PostgreSQL

Check POSTGRES_DSN, the database container, and host port 5433.

docker compose -f deploy/docker-compose.yml ps

Compose says elitegate_net does not exist

Create it:

docker network create elitegate_net

Gateway fails during startup

Check:

PROJECT_ID is set;

the project exists;

Admin API is reachable from the gateway;

gateway sync credentials are valid;

the project has valid routing configuration.

Upstream works in browser but not through gateway

The target must be reachable from the gateway environment.

Instead of:

http://localhost:8085

use a reachable Docker DNS name, host gateway address, private IP or resolvable internal hostname.

Dynamic gateway is healthy but not publicly reachable

The current project does not complete the full per-gateway ALB/DNS public endpoint lifecycle automatically.

Custom-domain provisioning does not progress

Check:

AWS automation setting;

region;

ALB listener ARN;

Worker IAM permissions;

DNS ownership/validation records;

routing DNS.

Documentation

For deeper implementation documentation, keep detailed material outside the main README.

Recommended documents:

docs/
├── ELITEGATE_COMPLETE_DOCUMENTATION.md
├── ARCHITECTURE.md
├── API.md
├── AUTHENTICATION.md
├── MULTI_TENANCY.md
├── DEPLOYMENT.md
├── SECURITY.md
└── TROUBLESHOOTING.md

If present in the repository, see Complete EliteGate Documentation.

Contributing

Create a feature branch.

Make focused changes.

Run formatting and tests.

Verify tenant isolation for project-scoped features.

Do not commit secrets or production credentials.

Open a pull request with the behavior, tests and operational impact clearly described.

Recommended checks:

gofmt -w .
go vet ./...
go test ./... -race -count=1