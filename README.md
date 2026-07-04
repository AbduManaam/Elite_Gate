# CoreGuard Gateway

CoreGuard Gateway is a high-performance, enterprise-grade, multi-tenant API Gateway and control plane built with Go, PostgreSQL, and Redis. It is designed to proxy, secure, manage, and monitor incoming API traffic with minimal overhead, drawing structural and conceptual inspiration from industry leaders like Kong.

---

## 🏗️ Architecture & Component Overview

CoreGuard Gateway consists of several integrated services:

1. **Gateway Proxy Engine (`cmd/gateway`)**: 
   - The high-throughput HTTP/HTTPS proxy that receives external traffic.
   - Per-request validation chain: Rate limiting, Authentication (JWT/API Keys), IP Filtering, CORS, and Scope/Role Authorization.
   - Dynamic routing and upstream proxying using customizable load-balancing strategies (Round-Robin, Least Connections, IP Hash).
   - Dynamic configuration hot-reloading from the PostgreSQL database (polling/watcher based, manual reload triggering).

2. **Administration API (`cmd/admin`)**:
   - A secure REST control plane written in Gin, managing the gateway's core configurations.
   - Multi-tenant architecture using isolated **Projects**, secured with project-scoped membership roles (Owner, Editor, Viewer).
   - Automated provisioning and decommissioning of secondary gateway nodes using Docker Container Manager via Docker SDK.
   - Security features such as public self-service signup (`/admin/signup`), single-instance admin bootstrap (`/admin/register`), and automated IP allowlisting for admin endpoints.

3. **Token CLI Utility (`cmd/token`)**:
   - A command-line companion tool to sign and issue custom JWT tokens for developers and clients.

4. **Background Worker Service (`cmd/worker`)**:
   - Stub container handling delayed cleanup tasks, expired token pruning, and health-checking routines.

5. **Observability Stack (`deploy/docker-compose.yml`)**:
   - Centralized metrics capture via **Prometheus** and log aggregation via **Loki & Promtail**.
   - Visualization dashboards built into **Grafana**.

---

## 🚀 Key Features

- **Dynamic Ingress Routing**: Match requests by HTTP paths (exact or prefix match), HTTP methods, and protocol, forwarding traffic directly to configured upstream backends.
- **Upstream Load Balancing**: Define pools of upstream servers (targets) with adjustable weights, active backend health checking, and multiple load balancer strategies.
- **Traffic Control & Rate Limiting**: Limit API consumption globally or route-by-route using a Redis-backed token bucket/rate limiter.
- **Security & Access Control (CORS/Auth/RBAC)**:
  - CORS header injection per route/policy.
  - API Key rotation and revocation.
  - JWT token verification with granular Client ID, Role, and Scope checking.
  - Admin Console Role-Based Access Control (RBAC).
- **Docker-native Gateway Provisioning**: Dynamically scale or decommission gateway runtime containers directly from the Admin control plane.
- **Audit Logs Tracking**: Collapsible history of all configuration changes (created, modified, or deleted routes, upstreams, and policies) logged with diffs.

---

## 🛠️ Tech Stack

- **Language**: Go 1.25+
- **Databases**: PostgreSQL 16 (Configuration Store), Redis 7 (Rate limiting cache & session store)
- **Containerization**: Docker & Docker Compose
- **Metrics & Logging**: Prometheus, Loki, Promtail, Grafana
- **Frameworks**: Gin Gonic, Zerolog (observability), Viper/Gotenv (configuration)

---

## 🏃 Getting Started

### Prerequisites

Make sure you have the following installed on your machine:
- Go 1.25.4 or higher
- Docker and Docker Compose
- [Taskfile (Go Task)](https://taskfile.dev/) (Optional, but recommended for task automation)

### 1. Environment Configuration

Copy the default variables or configure your `.env` at the root directory:

```ini
# PostgreSQL
POSTGRES_DSN=postgres://coreguard_app:coreguard_app_pass@postgres:5432/elitegate_db?sslmode=disable

# Redis
REDIS_ADDR=redis:6379
REDIS_PASSWORD=redis_secret

# Gateway Ports & Intervals
GATEWAY_PORT=8080
ADMIN_PORT=9090
GRPC_GATEWAY_PORT=:50051
ROUTE_RELOAD_INTERVAL=10s

# JWT Security
JWT_SECRET=supersecretjwtkey_32byteslongkey!

# Environment
APP_ENV=development
```

### 2. Running with Task

We use `Taskfile.yml` to automate project commands. Run `task --list` to see all available targets.

#### Spin up the infrastructure stack:
```bash
# Start PostgreSQL, Redis, Prometheus, Loki, Grafana, and Promtail
task docker-up
```

#### Run services locally for development:
```bash
# Run the Gateway proxy engine
task run-gateway

# Run the Admin control plane API
task run-admin
```

#### Seed the database:
```bash
# Seed standard routes
task seed-routes

# Seed multi-tenant test projects and collaborators for Postman testing
task seed-test-projects
```

#### Other utility tasks:
```bash
# Run Go unit tests
task test

# Build all production binaries (saved into bin/)
task build-all

# Generate a client JWT token
task token CLIENT="test-client-id" ROLE="admin"
```

---

## 🛣️ Admin API Endpoint Reference

The Admin API runs by default on port `9090`. All `/admin/v1` routes require JWT authentication except where noted.

### Auth & Onboarding

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `POST` | `/admin/register` | Initialize the first Super Admin (fails if users exist) | No |
| `POST` | `/admin/signup` | Public self-service company tenant onboarding | No |
| `POST` | `/admin/login` | Log in and receive access/refresh tokens | No (Rate limited) |
| `POST` | `/admin/refresh` | Refresh an expired access token | No |
| `POST` | `/admin/logout` | Revoke active refresh token | Yes |

### Project Management

| Method | Endpoint | Description | Scope |
| :--- | :--- | :--- | :--- |
| `POST` | `/admin/v1/projects` | Create a new project | Super Admin / User |
| `GET` | `/admin/v1/projects` | List projects user belongs to | User |
| `PUT` | `/admin/v1/projects/:projectId` | Update project metadata | Project Scope |
| `DELETE` | `/admin/v1/projects/:projectId` | Delete project | Project Scope (Owner) |
| `POST` | `/admin/v1/reload` | Force cluster-wide reload of gateway caches | Super Admin |

### Project Scoped Configurations (`/admin/v1/projects/:projectId/...`)

#### 1. Routes
- `GET /routes` - List project routes (Viewer+)
- `POST /routes` - Create route (Editor+)
- `PUT /routes/:id` - Update route config (Editor+)
- `PATCH /routes/:id/disable` - Disable route (Editor+)
- `DELETE /routes/:id` - Delete route (Editor+)
- `POST /routes/:id/policy` - Assign security policy (Editor+)
- `DELETE /routes/:id/policy` - Remove security policy (Editor+)

#### 2. Upstreams & Targets
- `GET /upstreams` - List upstreams (Viewer+)
- `POST /upstreams` - Create upstream pool (Editor+)
- `GET /upstreams/:id/health` - Check upstream active targets health (Viewer+)
- `PUT /upstreams/:id` - Update upstream settings (Editor+)
- `PATCH /upstreams/:id/disable` - Disable upstream (Editor+)
- `DELETE /upstreams/:id` - Delete upstream (Editor+)
- `GET /upstreams/:id/targets` - List upstream instances/targets (Viewer+)
- `POST /upstreams/:id/targets` - Add target instance to upstream (Editor+)
- `DELETE /upstreams/:id/targets/:targetId` - Remove target instance (Editor+)

#### 3. Policies (Rate limits, CORS, ACL rules)
- `GET /policies` - List policies (Viewer+)
- `POST /policies` - Create policy (Editor+)
- `PUT /policies/:id` - Update policy (Editor+)
- `DELETE /policies/:id` - Delete policy (Owner)

#### 4. Project Members
- `GET /members` - List collaborators (Viewer+)
- `GET /members/lookup` - Lookup user email (Owner)
- `POST /members` - Invite member to project (Owner)
- `PUT /members/:memberId` - Update member's RBAC role (Owner)
- `DELETE /members/:memberId` - Remove member from project (Owner)

#### 5. API Keys
- `GET /keys` - List active client API keys (Viewer+)
- `POST /keys` - Generate new API key (Editor+)
- `POST /keys/:id/rotate` - Rotate client API key (Editor+)
- `DELETE /keys/:id` - Revoke API key (Editor+)

#### 6. Docker Gateways
- `GET /gateways` - List gateway instances assigned to the project (Viewer+)
- `POST /gateways` - Provision a new Gateway container (Editor+)
- `DELETE /gateways/:gatewayId` - Decommission Gateway container (Editor+)

#### 7. Audit Logs
- `GET /audit-logs` - Retrieve detailed changes history list (Viewer+)

---

## 📈 Observability & Monitoring

The CoreGuard platform exposes metric endpoints and ships internal logs dynamically:
- **Gateway Metrics**: `http://localhost:8080/metrics`
- **Admin Metrics**: `http://localhost:9090/metrics`
- **Grafana Panel**: Access at `http://localhost:3001` (Username: `admin`, Password: `admin` by default). Includes logs visualizer connected to Loki.
