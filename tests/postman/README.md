# EliteGate Multi-Tenant Postman Collection

This directory contains the Postman collection to validate the **Shared Gateway Model** and the **Dedicated Gateway Model** inside the **CoreGuard / EliteGate** API Gateway system.

## 📂 File Location
The Postman collection is saved at:
👉 [elitegate_multitenant_tests.postman_collection.json](file:///c:/Users/abdum/OneDrive/Desktop/New%20folder/Coding/CoreGuard%20Gateway/tests/postman/elitegate_multitenant_tests.postman_collection.json)

---

## 🛠️ Collection Structure

```
EliteGate Multi-Tenant API Gateway Tests
├── 1. Shared Gateway Model (Shared Everything)
│   ├── 1.1 Project & Member Administration
│   │   ├── Create Project (POST /admin/v1/projects)
│   │   ├── Add Project Member (POST /admin/v1/projects/:projectId/members)
│   │   └── List Project Members (GET /admin/v1/projects/:projectId/members)
│   ├── 1.2 Upstream & Route Management
│   │   ├── Create Upstream (POST /admin/v1/projects/:projectId/upstreams)
│   │   ├── List Upstreams (GET /admin/v1/projects/:projectId/upstreams)
│   │   ├── Create Route (POST /admin/v1/projects/:projectId/routes)
│   │   ├── Update Route (PUT /admin/v1/projects/:projectId/routes/:id)
│   │   └── Delete Route (DELETE /admin/v1/projects/:projectId/routes/:id)
│   ├── 1.3 Security Policies & API Keys
│   │   ├── Create Policy (POST /admin/v1/projects/:projectId/policies)
│   │   ├── List Policies (GET /admin/v1/projects/:projectId/policies)
│   │   └── Create API Key (POST /admin/v1/projects/:projectId/keys)
│   ├── 1.4 Gateway Traffic & Verification
│   │   ├── Gateway Request - Success (/api/users)
│   │   ├── Gateway Request - Unauthorized (Missing credentials)
│   │   ├── Gateway Request - Rate Limit Exhaustion (429 verification)
│   │   └── Verify Audit Log Action (GET /admin/v1/projects/:projectId/audit-logs)
│   └── 1.5 Resiliency & Configuration Sync
│       └── Verify Route Reload Synchronization (GET /ready)
│
└── 2. Dedicated Gateway Model
    ├── 2.1 Provisioning & Lifecycle
    │   ├── Create Project for Dedicated Gateway (POST /admin/v1/projects)
    │   ├── Provision Dedicated Gateway (POST /admin/v1/provisioning/gateways)
    │   ├── Register Gateway Instance (POST /admin/v1/provisioning/gateways/register)
    │   └── Dedicated Gateway Health Check (GET http://localhost:8081/health)
    ├── 2.2 Isolated Route & Config Verification
    │   ├── Create Upstream for Dedicated Project (POST /admin/v1/projects/:projectId/upstreams)
    │   └── Create Route for Dedicated Project (POST /admin/v1/projects/:projectId/routes)
    └── 2.3 Isolated Traffic Verification
        ├── Route Request via Gateway A (GET http://localhost:8081/api/users)
        ├── Cross-Project Isolation Check (GET http://localhost:8081/api/tenant-c/users -> Expect 404)
        └── Verify Recovery Configuration Sync (GET http://localhost:8081/api/users)
```

---

## 🔑 Environment Variables

The collection includes a pre-configured list of variables. You can override these in your Postman Environment:

| Variable | Default Value | Description |
| :--- | :--- | :--- |
| `admin_url` | `http://localhost:9090` | Admin Control Plane endpoint. |
| `gateway_url` | `http://localhost:8080` | Shared Data Plane gateway endpoint. |
| `token` | *(Auto-generated)* | JWT Token obtained from authentication. |
| `project_id` | `00000000-0000-0000-0000-000000000000` | Current isolated project context ID. |
| `upstream_id` | *(Auto-populated)* | ID of the created project upstream. |
| `route_id` | *(Auto-populated)* | ID of the created project route. |
| `policy_id` | *(Auto-populated)* | ID of the created project policy. |
| `collaborator_user_id` | `22222222-2222-2222-2222-222222222222` | Seeded admin user to add to the project. |

---

## 🏃 Request Flows & Verification Logic

### Pattern 1: Shared Gateway Model Workflow

1. **Administration Setup:**
   * **Create Project:** Establishes `tenant-c` context. The test script saves the generated UUID as `project_id`.
   * **Add Project Member:** Grants roles (`editor`/`viewer`) in `project_members` to verify multi-admin permissions.
2. **Resource Management:**
   * **Create Upstream & Route:** Creates upstreams/routes scoped to `project_id`.
   * **Update & Delete Route:** Modifies route options dynamically to verify state hot-swaps.
3. **Security Scenarios:**
   * **Create Policy:** Sets an auth policy and rate-limits to `10 RPM`.
   * **Create API Key:** Generates a cryptographic token to authenticate traffic.
4. **Traffic Routing & Rate Limits:**
   * **Success traffic:** Verify `/api/users` redirects to target services with status `200 OK`.
   * **Unauthorized check:** Confirm headers block without tokens.
   * **Rate-limiting check:** Send requests rapidly. Postman verifies that headers `X-RateLimit-Limit`, `X-RateLimit-Remaining: 0`, and status `429` are hit when limits are crossed.
5. **Auditing & Resiliency:**
   * **Audit Log verification:** Confirms database captures metadata and logs audit trails.
   * **Config Sync check:** Hits `/ready` to inspect if the background loader is running and dynamic configs are in sync.

### Pattern 2: Dedicated Gateway Model Workflow

1. **Orchestration Lifecycle:**
   * **Create Project & Provision Gateway:** Signals the platform to provision a dedicated gateway container.
   * **Register Gateway:** Registers the routing IP/port details (`localhost:8081`).
   * **Health Check:** Hits `http://localhost:8081/health` to confirm the gateway container is active.
2. **Routing Isolation:**
   * **Isolated configuration:** Adds a route with a clean path `/api/users` to the database.
   * **Traffic Match Success:** Hit `http://localhost:8081/api/users` $\rightarrow$ succeeds because the route belongs to its own scoped `PROJECT_ID`.
   * **Cross-Project Isolation Check:** Hit `http://localhost:8081/api/tenant-c/users` $\rightarrow$ returns **`404 Not Found`** since this dedicated gateway has no context of `tenant-c` routes.
3. **Resiliency:**
   * **Recovery check:** Restarts the container and verifies configuration reloads automatically from database.
