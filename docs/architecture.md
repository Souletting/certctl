# Certctl Architecture

## Overview

Certctl is a certificate management platform with a **decoupled control-plane and agent architecture**. The control plane orchestrates certificate issuance and renewal, while stateless agents deployed across your infrastructure handle certificate generation, deployment, and renewal without exposing private keys to the control plane.

### Design Principles

1. **Zero Private Key Exposure** — Private keys generated and managed only on agents
2. **Decoupled Operations** — Agents operate autonomously; control plane is optional for agent function
3. **Audit-First** — Complete traceability of all issuance, deployment, and rotation events
4. **Connector Architecture** — Pluggable issuers, targets, and notifiers for extensibility
5. **Self-Hosted** — No cloud lock-in; run on Kubernetes, Docker, or bare metal

---

## System Components

### Control Plane

The control plane is a REST API server backed by PostgreSQL. It:

- **Manages state**: Certificates, agents, targets, issuers, policies
- **Orchestrates issuance**: Coordinates with ACME/PKI issuers
- **Tracks jobs**: Certificate issuance, renewal, and deployment workflows
- **Audits all actions**: Immutable audit trail for compliance
- **Dispatches work**: Schedules renewal checks and deployment jobs

**Deployment Options**: Single binary, Docker container, Kubernetes deployment

### Agents

Lightweight agents deployed on or near your infrastructure. They:

- **Generate certificates**: Create private keys and certificate requests
- **Deploy certificates**: Push certs to NGINX, F5, IIS, etc.
- **Manage credentials**: Store and rotate API keys with control plane
- **Report status**: Health checks and job completion status
- **Operate independently**: Continue functioning even if control plane is unreachable

**Deployment Options**: Container, systemd service, Kubernetes DaemonSet, Lambda

### PostgreSQL Database

Persistent state store:

```
├── Teams & Ownership
│  ├── teams
│  └── owners
├── Certificate Management
│  ├── certificates
│  ├── certificate_versions
│  └── renewal_policies
├── Infrastructure
│  ├── agents
│  ├── targets
│  └── target_connections
├── Issuance
│  ├── issuers
│  ├── jobs
│  └── job_steps
├── Monitoring & Audit
│  ├── audit_logs
│  ├── notifications
│  └── deployment_history
└── Configuration
   ├── agent_api_keys
   └── connector_config
```

---

## Data Flow: Certificate Lifecycle

### 1. **Create Managed Certificate**

```
User/API
   │
   ├─→ POST /api/v1/certificates
   │    {
   │      "domain": "api.example.com",
   │      "issuer_id": "issuer-001",
   │      "target_ids": ["nginx-prod-01"],
   │      "renewal_days_before": 30
   │    }
   │
   └─→ Control Plane
        ├─ Insert certificate record
        ├─ Create initial job
        ├─ Log audit event
        └─ Return cert ID + API response
```

### 2. **Agent Requests Certificate (CSR → Issuance)**

```
Agent                          Control Plane                    ACME Issuer
  │                                  │                               │
  ├─ POST /api/v1/csr               │                               │
  │  {                              │                               │
  │    "cert_id": "cert-123",       │                               │
  │    "csr": "-----BEGIN CSR..."   │                               │
  │  }                              │                               │
  │                                 ├─ Validate CSR                 │
  │                                 │                               │
  │                                 ├─ POST /directory/new-order    │
  │                                 ├──────────────────────────────→
  │                                 │                               │
  │                                 │← Poll challenges              │
  │                                 ├──────────────────────────────→
  │                                 │                               │
  │                                 ├─ POST /acme/finalize         │
  │                                 ├──────────────────────────────→
  │                                 │                               │
  │← Certificate + chain           │← Signed certificate           │
  ├─────────────────────────────────│                               │
  │                                 │                               │
  ├─ Store locally:                │                               │
  │  /etc/certctl/api.example.com/  │                               │
  │   ├─ cert.pem                   │                               │
  │   ├─ key.pem (never sent back)  │                               │
  │   └─ chain.pem                  │                               │
  │                                 │                               │
  └─ POST /api/v1/deployments      │                               │
     { "cert_id", "status": "ok" }  │                               │
                                    ├─ Update cert record           │
                                    ├─ Log "issued" event           │
                                    └─ Trigger deployment jobs      │
```

### 3. **Deploy Certificate to Target**

```
Agent                          Target System
  │
  ├─ Fetch target credentials from config
  │
  ├─ Load certificate:
  │  - /etc/certctl/api.example.com/cert.pem
  │  - /etc/certctl/api.example.com/key.pem
  │
  ├─ NGINX (SSH):
  │  ├─ scp cert.pem → /etc/nginx/ssl/
  │  ├─ scp key.pem → /etc/nginx/ssl/ (restricted perms)
  │  ├─ ssh nginx -s reload
  │  └─ Verify: curl https://api.example.com/health
  │
  ├─ F5 (HTTPS API):
  │  ├─ Authenticate with credentials
  │  ├─ POST /mgmt/tm/ltm/cert {"name": "api.example.com", "cert": "..."}
  │  ├─ PUT /mgmt/tm/ltm/virtual (update virtual server)
  │  └─ Verify: F5 configuration updated
  │
  ├─ IIS (WinRM):
  │  ├─ Import cert to store: Import-PfxCertificate
  │  ├─ Bind to site: Set-WebBinding
  │  └─ Verify: Get-WebBinding
  │
  └─ Report deployment status:
     POST /api/v1/deployments/{id}/status
     { "status": "success", "deployed_at": "..." }
```

### 4. **Renewal Check & Rotation**

```
Scheduler (Control Plane)
  │
  ├─ Every hour: SELECT certificates WHERE expiry_date < NOW() + 30 days
  │
  ├─ For each certificate:
  │  │
  │  ├─ Create renewal job
  │  ├─ Notify agent(s)
  │  │
  │  └─ Agent flow:
  │     ├─ Generate new CSR
  │     ├─ Request new certificate
  │     ├─ Deploy new cert to targets
  │     ├─ Verify deployment
  │     └─ Delete old private key from agent
  │
  ├─ Log completion
  └─ Notify via email/webhook
```

---

## Connector Architecture

Certctl uses **connector interfaces** for extensibility. Connectors are pluggable implementations of specific capabilities.

### Issuer Connector

Handles certificate issuance from external PKI systems.

```go
type IssuerConnector interface {
    // GetDirectory returns the ACME directory
    GetDirectory(ctx context.Context) (*ACMEDirectory, error)

    // NewAccount registers a new account
    NewAccount(ctx context.Context, email string) (*Account, error)

    // NewOrder creates a new certificate order
    NewOrder(ctx context.Context, identifiers []Identifier) (*Order, error)

    // GetAuthorization retrieves challenge info
    GetAuthorization(ctx context.Context, authURL string) (*Authorization, error)

    // FinalizeOrder submits CSR and gets certificate
    FinalizeOrder(ctx context.Context, orderURL, csr string) ([]byte, error)
}
```

**Built-in Issuers**:
- `acme` — ACME v2 protocol (Let's Encrypt, Sectigo, etc.)

**Example Usage**:
```yaml
issuer:
  type: acme
  config:
    directory_url: https://acme-v02.api.letsencrypt.org/directory
    email: admin@example.com
```

### Target Connector

Deploys certificates to infrastructure systems.

```go
type TargetConnector interface {
    // Validate tests connectivity and credentials
    Validate(ctx context.Context) error

    // Deploy pushes certificate to target
    Deploy(ctx context.Context, cert *Certificate) error

    // Remove removes/revokes certificate from target
    Remove(ctx context.Context, domain string) error

    // GetStatus checks deployment status
    GetStatus(ctx context.Context, domain string) (string, error)
}
```

**Built-in Targets**:
- `nginx` — NGINX via SSH
- `f5` — F5 BIG-IP via REST API
- `iis` — Microsoft IIS via WinRM

**Example Usage**:
```yaml
target:
  type: nginx
  config:
    host: web01.prod.internal
    ssh_user: deploy
    ssh_key: /etc/certctl/keys/deploy.pem
    cert_path: /etc/nginx/ssl
```

### Notifier Connector

Sends notifications about certificate events.

```go
type NotifierConnector interface {
    // Send delivers a notification
    Send(ctx context.Context, notif *Notification) error

    // Validate checks configuration
    Validate(ctx context.Context) error
}
```

**Built-in Notifiers**:
- `email` — SMTP email
- `webhook` — HTTP webhooks

**Example Usage**:
```yaml
notifier:
  type: email
  config:
    smtp_host: smtp.example.com
    smtp_port: 587
    username: alerts@example.com
    password: "***"
    from_address: certctl@example.com
    recipients:
      - ops@example.com
      - security@example.com
```

---

## Job Lifecycle & States

Jobs represent work to be done: certificate issuance, renewal, deployment, etc.

```
┌──────────┐
│ PENDING  │  Job created, waiting to be processed
└────┬─────┘
     │
     ↓
┌──────────┐
│ RUNNING  │  Job in progress (CSR generation, issuance, deployment)
└────┬─────┘
     │
     ├─→ SUCCESS ──→ COMPLETED (job done, no errors)
     │
     ├─→ FAILURE ──→ FAILED (error occurred, may retry)
     │
     └─→ CANCEL ───→ CANCELLED (user or scheduler cancelled)

Additional states:
  • RETRY_WAIT  — Backoff before retry
  • ABANDONED   — Max retries exceeded
```

### Job Steps

Complex jobs are broken into steps:

```
Issuance Job
  │
  ├─ Step 1: Notify agent of CSR request
  │   Status: COMPLETED
  │
  ├─ Step 2: Wait for CSR from agent
  │   Status: RUNNING (timeout: 5 min)
  │
  ├─ Step 3: Submit to ACME issuer
  │   Status: PENDING
  │
  ├─ Step 4: Poll for certificate
  │   Status: PENDING
  │
  └─ Step 5: Trigger deployment jobs
      Status: PENDING
```

---

## Security Model

### Private Key Management

```
Private Key Lifecycle
  │
  ├─ GENERATED on Agent (never sent to control plane)
  │  └─ Location: /etc/certctl/domains/{domain}/key.pem
  │
  ├─ STORED on Agent
  │  ├─ File permissions: 0600 (agent user only)
  │  └─ Encrypted at rest (optional, per deployment)
  │
  ├─ USED on Agent for:
  │  ├─ Deployment to targets
  │  └─ Certificate renewal
  │
  └─ DELETED on Agent
     ├─ Old key deleted after successful renewal
     └─ Manual revocation on agent removal
```

### Authentication & Authorization

**Agent-to-Server**:
- API Key (registered at agent creation)
- mTLS optional for high-security deployments
- All API calls include agent ID + API key

**Server-to-External Systems**:
- ACME: ACME protocol with account key
- NGINX: SSH key authentication
- F5: Username/password or token
- IIS: WinRM with encrypted credentials

### Audit Logging

Every action is logged:

```json
{
  "id": "audit-98765",
  "timestamp": "2024-03-14T10:30:00Z",
  "actor": {
    "type": "agent",
    "id": "agent-prod-01"
  },
  "action": "certificate_issued",
  "resource": {
    "type": "certificate",
    "id": "cert-api-example-com"
  },
  "status": "success",
  "details": {
    "issuer": "acme/letsencrypt",
    "expiry": "2024-06-12T10:30:00Z",
    "deployed_to": ["nginx-prod-01"]
  }
}
```

**Query examples**:
- All actions by agent: `GET /audit/logs?actor_type=agent&actor_id=agent-001`
- All deployments: `GET /audit/logs?action=certificate_deployed`
- Last 30 days: `GET /audit/logs?from=2024-02-12`

### Data Encryption at Rest

Optional encryption for sensitive fields:

- Passwords in connector configs
- API keys
- ACME account keys

Uses AES-256-GCM with per-row nonce.

---

## Scaling Considerations

### Control Plane Scaling

**Single Server Limits**:
- ~1000 agents (verified in testing)
- ~10,000 managed certificates
- ~100,000 audit log entries per day

**Horizontal Scaling** (future):
- Multiple server instances behind load balancer
- Shared PostgreSQL backend
- Distributed job queue (Redis/RabbitMQ)

### Agent Scaling

Agents are stateless and scale horizontally:

- Each agent processes certificates independently
- Scheduler distributes renewal checks across agents
- No inter-agent communication required

### Database Scaling

For large deployments:
- Vertical scaling: More CPU/RAM for PostgreSQL
- Read replicas: For audit log queries
- Partitioning: Audit logs by date
- Connection pooling: PgBouncer

---

## Integration Points

### External Integrations

```
Certctl
  │
  ├─→ ACME Servers
  │   ├─ Let's Encrypt
  │   ├─ Sectigo
  │   └─ Internal ACME (optional)
  │
  ├─→ Infrastructure Targets
  │   ├─ NGINX (SSH)
  │   ├─ F5 (REST API)
  │   ├─ IIS (WinRM)
  │   └─ Kubernetes (future)
  │
  ├─→ Notification Systems
  │   ├─ SMTP (email)
  │   ├─ HTTP webhooks
  │   └─ Slack (future)
  │
  └─→ External Systems
      ├─ Vault (credential storage)
      ├─ HashiCorp Consul (service discovery)
      └─ Prometheus (metrics)
```

### Internal Component Communication

```
Agent ← → Control Plane
  ├─ Agent registration
  ├─ CSR submission
  ├─ Certificate retrieval
  ├─ Deployment status
  └─ Health checks (bidirectional)

Scheduler → Services
  ├─ Certificate renewal
  ├─ Job processing
  ├─ Notifications
  └─ Cleanup tasks
```

---

## Deployment Topologies

### Single-Node (Development)

```
┌────────────────────────────┐
│ Server + Agent             │
│ ├─ HTTP API (8443)         │
│ ├─ PostgreSQL              │
│ └─ Agent (test mode)       │
└────────────────────────────┘
```

### Docker Compose (Local Dev)

```
┌─────────────────────────────────────┐
│ Docker Network                      │
│ ├─ certctl-server (8443)            │
│ ├─ postgres (5432)                  │
│ ├─ certctl-agent (managed)          │
│ └─ pgadmin (5050, optional)         │
└─────────────────────────────────────┘
```

### Kubernetes (Production)

```
┌──────────────────────────────────────────────────┐
│ Kubernetes Cluster                               │
│ ├─ Deployment: certctl-server (replicas=3)       │
│ ├─ DaemonSet: certctl-agent (all nodes)          │
│ ├─ StatefulSet: postgres (primary + replica)     │
│ ├─ ConfigMap: connector configurations           │
│ └─ Secret: API keys, credentials                 │
└──────────────────────────────────────────────────┘
```

---

## Performance Characteristics

| Operation | Typical Duration | Bottleneck |
|-----------|------------------|-----------|
| Certificate request (CSR) | 100-500ms | Agent network latency |
| ACME challenge (DNS) | 30-60s | DNS propagation |
| ACME finalize | 1-5s | ACME server |
| NGINX deployment | 500ms-2s | SSH latency + nginx reload |
| F5 deployment | 2-10s | F5 API response |
| IIS deployment | 3-15s | WinRM latency |

---

## Future Enhancements

- **HSM Support**: Hardware security module integration for ACME account keys
- **Multi-Region**: Control plane federation with local agents
- **HA Control Plane**: Active-active with etcd-backed state
- **Policy Engine**: Advanced renewal and deployment policies
- **Certificate Pinning**: HPKP and pin validation
- **Metrics**: Prometheus integration for observability

---

See [README.md](../README.md) for quick start and [docs/](../) for additional guides.
