# Certctl Connector Development Guide

Connectors extend certctl to integrate with external systems for certificate issuance, deployment, and notifications. This guide covers building custom connectors from scratch.

## Overview

Three types of connectors:

1. **IssuerConnector** — Obtains certificates from PKI systems (ACME, Vault, DigiCert)
2. **TargetConnector** — Deploys certificates to infrastructure (NGINX, F5, IIS, Kubernetes)
3. **NotifierConnector** — Sends notifications about certificate events (Email, Webhooks, Slack)

All connectors:
- Are registered with a unique type identifier
- Accept configuration at initialization
- Are used by the control plane or agents
- Are tested via validation endpoints

---

## IssuerConnector Interface

Issuers obtain certificates from external PKI systems.

### Interface Definition

```go
package issuer

type IssuerConnector interface {
    // Validate checks the issuer configuration and connectivity
    Validate(ctx context.Context) error

    // IssueCertificate requests a certificate for the given domains
    IssueCertificate(ctx context.Context, req *IssueRequest) (*CertificateResponse, error)

    // RevokeCertificate revokes an issued certificate
    RevokeCertificate(ctx context.Context, certPEM []byte) error

    // GetStatus returns the status of an issuance request
    GetStatus(ctx context.Context, requestID string) (*StatusResponse, error)
}

type IssueRequest struct {
    Domains          []string      // Primary domain + SANs
    CSR              []byte        // Certificate Signing Request (PEM)
    ValidityDays     int           // Requested validity period
    NotBefore        *time.Time    // Optional: not valid before
    NotAfter         *time.Time    // Optional: not valid after
    Metadata         map[string]string
}

type CertificateResponse struct {
    Certificate      []byte        // Signed certificate (PEM)
    CertificateChain []byte        // CA chain (PEM)
    RequestID        string        // For status tracking
    ExpiresAt        time.Time
    IssuedAt         time.Time
}
```

### Example: Vault PKI Issuer

```go
package vault

import (
    "context"
    "crypto/x509"
    "encoding/pem"
    "fmt"
    "github.com/hashicorp/vault/api"
)

type VaultConfig struct {
    Address   string
    Token     string
    PKIPath   string // e.g., "pki"
    RoleName  string // e.g., "example-dot-com"
}

type VaultIssuer struct {
    config *VaultConfig
    client *api.Client
}

func New(cfg *VaultConfig) (*VaultIssuer, error) {
    client, err := api.NewClient(&api.Config{Address: cfg.Address})
    if err != nil {
        return nil, err
    }
    client.SetToken(cfg.Token)
    return &VaultIssuer{config: cfg, client: client}, nil
}

// Validate tests connectivity and access
func (v *VaultIssuer) Validate(ctx context.Context) error {
    _, err := v.client.Auth().Token().LookupSelf()
    if err != nil {
        return fmt.Errorf("Vault auth failed: %w", err)
    }
    return nil
}

// IssueCertificate requests a certificate from Vault
func (v *VaultIssuer) IssueCertificate(ctx context.Context, req *issuer.IssueRequest) (
    *issuer.CertificateResponse, error) {

    // Extract primary domain and SANs
    if len(req.Domains) == 0 {
        return nil, fmt.Errorf("no domains provided")
    }
    primaryDomain := req.Domains[0]
    altNames := req.Domains[1:]

    // Decode CSR
    csrBlock, _ := pem.Decode(req.CSR)
    if csrBlock == nil {
        return nil, fmt.Errorf("invalid CSR format")
    }
    csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
    if err != nil {
        return nil, err
    }

    // Call Vault PKI issue endpoint
    path := fmt.Sprintf("%s/issue/%s", v.config.PKIPath, v.config.RoleName)
    data := map[string]interface{}{
        "common_name":       primaryDomain,
        "alt_names":         altNames,
        "ttl":               fmt.Sprintf("%dh", req.ValidityDays*24),
        "private_key_format": "pem",
    }

    secret, err := v.client.Logical().Write(path, data)
    if err != nil {
        return nil, fmt.Errorf("Vault issue failed: %w", err)
    }

    // Extract certificate and chain
    certPEM := secret.Data["certificate"].(string)
    chainPEM := secret.Data["ca_chain"].([]interface{})
    caChain := ""
    for _, ca := range chainPEM {
        caChain += ca.(string) + "\n"
    }

    return &issuer.CertificateResponse{
        Certificate:      []byte(certPEM),
        CertificateChain: []byte(caChain),
        RequestID:        secret.Data["request_id"].(string),
        ExpiresAt:        time.Now().AddDate(0, 0, req.ValidityDays),
        IssuedAt:         time.Now(),
    }, nil
}

// RevokeCertificate revokes a certificate in Vault
func (v *VaultIssuer) RevokeCertificate(ctx context.Context, certPEM []byte) error {
    certBlock, _ := pem.Decode(certPEM)
    if certBlock == nil {
        return fmt.Errorf("invalid certificate format")
    }
    cert, err := x509.ParseCertificate(certBlock.Bytes)
    if err != nil {
        return err
    }

    path := fmt.Sprintf("%s/revoke", v.config.PKIPath)
    _, err = v.client.Logical().Write(path, map[string]interface{}{
        "certificate": cert.SerialNumber.String(),
    })
    return err
}

// GetStatus returns the status of an issuance request
func (v *VaultIssuer) GetStatus(ctx context.Context, requestID string) (
    *issuer.StatusResponse, error) {
    // Vault PKI doesn't have a request status endpoint
    // Return immediate success (Vault issues synchronously)
    return &issuer.StatusResponse{
        Status:   "success",
        Ready:    true,
        IssuedAt: time.Now(),
    }, nil
}
```

### Registration

Register your issuer in the connector registry:

```go
// internal/connector/issuer/registry.go

package issuer

var registry = map[string]Factory{
    "acme":   func(cfg Config) (IssuerConnector, error) { return acme.New(&cfg) },
    "vault":  func(cfg Config) (IssuerConnector, error) { return vault.New(&cfg) },
    // Add more issuers here
}

func GetConnector(connectorType string, config Config) (IssuerConnector, error) {
    factory, ok := registry[connectorType]
    if !ok {
        return nil, fmt.Errorf("unknown issuer type: %s", connectorType)
    }
    return factory(config)
}
```

---

## TargetConnector Interface

Targets deploy certificates to infrastructure.

### Interface Definition

```go
package target

type TargetConnector interface {
    // Validate tests connectivity and credentials
    Validate(ctx context.Context) error

    // Deploy pushes the certificate to the target
    Deploy(ctx context.Context, req *DeployRequest) (*DeployResponse, error)

    // Remove removes/revokes a certificate from the target
    Remove(ctx context.Context, domain string) error

    // GetStatus checks the deployment status
    GetStatus(ctx context.Context, domain string) (*StatusResponse, error)
}

type DeployRequest struct {
    Domain           string        // Primary domain
    Certificate      []byte        // Signed certificate (PEM)
    PrivateKey       []byte        // Private key (PEM) - optional
    CertificateChain []byte        // CA chain (PEM)
    Metadata         map[string]string
}

type DeployResponse struct {
    RequestID   string
    Status      string // "success", "pending", "error"
    Message     string
    DeployedAt  time.Time
}

type StatusResponse struct {
    Status     string    // "deployed", "pending", "failed"
    DeployedAt time.Time
    Error      string
}
```

### Example: Custom Load Balancer Target

```go
package lb

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

type LBConfig struct {
    Host     string // Load balancer hostname
    Port     int    // HTTPS API port
    Username string
    Password string
}

type LoadBalancerTarget struct {
    config *LBConfig
    client *http.Client
}

func New(cfg *LBConfig) *LoadBalancerTarget {
    return &LoadBalancerTarget{
        config: cfg,
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

// Validate tests connectivity
func (lb *LoadBalancerTarget) Validate(ctx context.Context) error {
    req, err := http.NewRequestWithContext(ctx, "GET",
        fmt.Sprintf("https://%s:%d/api/health", lb.config.Host, lb.config.Port), nil)
    if err != nil {
        return err
    }
    req.SetBasicAuth(lb.config.Username, lb.config.Password)

    resp, err := lb.client.Do(req)
    if err != nil {
        return fmt.Errorf("load balancer unreachable: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("load balancer returned %d", resp.StatusCode)
    }
    return nil
}

// Deploy pushes certificate to the load balancer
func (lb *LoadBalancerTarget) Deploy(ctx context.Context, req *target.DeployRequest) (
    *target.DeployResponse, error) {

    body := map[string]interface{}{
        "domain":      req.Domain,
        "certificate": string(req.Certificate),
        "chain":       string(req.CertificateChain),
        "key":         string(req.PrivateKey),
    }

    payload, err := json.Marshal(body)
    if err != nil {
        return nil, err
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST",
        fmt.Sprintf("https://%s:%d/api/certs/upload", lb.config.Host, lb.config.Port),
        bytes.NewReader(payload))
    if err != nil {
        return nil, err
    }
    httpReq.SetBasicAuth(lb.config.Username, lb.config.Password)
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := lb.client.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("deployment failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("deployment returned %d", resp.StatusCode)
    }

    return &target.DeployResponse{
        Status:     "success",
        DeployedAt: time.Now(),
    }, nil
}

// Remove deletes a certificate from the load balancer
func (lb *LoadBalancerTarget) Remove(ctx context.Context, domain string) error {
    req, err := http.NewRequestWithContext(ctx, "DELETE",
        fmt.Sprintf("https://%s:%d/api/certs/%s", lb.config.Host, lb.config.Port, domain), nil)
    if err != nil {
        return err
    }
    req.SetBasicAuth(lb.config.Username, lb.config.Password)

    resp, err := lb.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("removal returned %d", resp.StatusCode)
    }
    return nil
}

// GetStatus checks deployment status
func (lb *LoadBalancerTarget) GetStatus(ctx context.Context, domain string) (
    *target.StatusResponse, error) {

    req, err := http.NewRequestWithContext(ctx, "GET",
        fmt.Sprintf("https://%s:%d/api/certs/%s/status", lb.config.Host, lb.config.Port, domain), nil)
    if err != nil {
        return nil, err
    }
    req.SetBasicAuth(lb.config.Username, lb.config.Password)

    resp, err := lb.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)

    return &target.StatusResponse{
        Status: result["status"].(string),
    }, nil
}
```

---

## NotifierConnector Interface

Notifiers send alerts about certificate events.

### Interface Definition

```go
package notifier

type NotifierConnector interface {
    // Validate checks configuration
    Validate(ctx context.Context) error

    // Send delivers a notification
    Send(ctx context.Context, notification *Notification) error
}

type Notification struct {
    EventType string // "certificate_issued", "renewal_failed", "deployment_success"
    Subject   string
    Body      string
    Severity  string // "info", "warning", "error"
    Metadata  map[string]string
}
```

### Example: Slack Notifier

```go
package slack

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

type SlackConfig struct {
    WebhookURL string // Slack incoming webhook URL
    Channel    string // Optional: override channel
    Username   string // Bot username
}

type SlackNotifier struct {
    config *SlackConfig
    client *http.Client
}

func New(cfg *SlackConfig) *SlackNotifier {
    return &SlackNotifier{
        config: cfg,
        client: &http.Client{Timeout: 10 * time.Second},
    }
}

// Validate checks webhook connectivity
func (s *SlackNotifier) Validate(ctx context.Context) error {
    payload := map[string]interface{}{
        "text": "Certctl test message",
    }
    data, _ := json.Marshal(payload)

    resp, err := s.client.Post(s.config.WebhookURL, "application/json", bytes.NewReader(data))
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
    }
    return nil
}

// Send posts a message to Slack
func (s *SlackNotifier) Send(ctx context.Context, notif *notifier.Notification) error {
    color := "good"
    if notif.Severity == "error" {
        color = "danger"
    } else if notif.Severity == "warning" {
        color = "warning"
    }

    payload := map[string]interface{}{
        "username": s.config.Username,
        "attachments": []map[string]interface{}{
            {
                "title":  notif.Subject,
                "text":   notif.Body,
                "color":  color,
                "fields": formatMetadata(notif.Metadata),
            },
        },
    }

    data, _ := json.Marshal(payload)
    resp, err := s.client.Post(s.config.WebhookURL, "application/json", bytes.NewReader(data))
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("slack post failed: %d", resp.StatusCode)
    }
    return nil
}

func formatMetadata(m map[string]string) []map[string]interface{} {
    fields := []map[string]interface{}{}
    for k, v := range m {
        fields = append(fields, map[string]interface{}{
            "title": k,
            "value": v,
            "short": true,
        })
    }
    return fields
}
```

---

## Testing Connectors

### Unit Tests

```go
package vault

import (
    "context"
    "testing"
)

func TestValidate(t *testing.T) {
    cfg := &VaultConfig{
        Address: "http://localhost:8200",
        Token:   "test-token",
    }
    issuer := New(cfg)

    err := issuer.Validate(context.Background())
    if err == nil {
        t.Fatal("expected error for invalid token")
    }
}

func TestIssueCertificate(t *testing.T) {
    // Mock Vault responses or use Vault test harness
    cfg := &VaultConfig{/* ... */}
    issuer := New(cfg)

    req := &issuer.IssueRequest{
        Domains:      []string{"example.com"},
        CSR:          testCSR,
        ValidityDays: 90,
    }

    resp, err := issuer.IssueCertificate(context.Background(), req)
    if err != nil {
        t.Fatalf("issuance failed: %v", err)
    }

    if len(resp.Certificate) == 0 {
        t.Fatal("no certificate returned")
    }
}
```

### Integration Tests

```bash
# Start dependent service
docker run -d --name vault -p 8200:8200 vault:latest server -dev

# Run tests
go test -tags=integration ./internal/connector/issuer/vault

# Cleanup
docker rm -f vault
```

### Validation Endpoints

Test connectors via the API:

```bash
# Validate an issuer
curl -X POST http://localhost:8443/api/v1/issuers/validate \
  -H "Content-Type: application/json" \
  -d '{
    "type": "vault",
    "config": {
      "address": "http://vault.example.com:8200",
      "token": "s.xxxxxxx"
    }
  }'

# Validate a target
curl -X POST http://localhost:8443/api/v1/targets/validate \
  -H "Content-Type: application/json" \
  -d '{
    "type": "nginx",
    "config": {
      "host": "web01.example.com",
      "ssh_user": "deploy"
    }
  }'
```

---

## Registering Custom Connectors

### 1. Create Connector Package

```
internal/connector/issuer/myissuer/
├── issuer.go       # Implementation
└── config.go       # Configuration validation
```

### 2. Implement Interface

```go
package myissuer

type MyIssuer struct {
    config *Config
}

func (m *MyIssuer) Validate(ctx context.Context) error {
    // Validation logic
}

func (m *MyIssuer) IssueCertificate(ctx context.Context, req *issuer.IssueRequest) (*issuer.CertificateResponse, error) {
    // Issuance logic
}
```

### 3. Register in Factory

```go
// internal/connector/issuer/factory.go

import "github.com/shankar0123/certctl/internal/connector/issuer/myissuer"

var factories = map[string]ConnectorFactory{
    "myissuer": func(cfg interface{}) (IssuerConnector, error) {
        return myissuer.New(cfg.(*myissuer.Config))
    },
}
```

### 4. Add Configuration Schema

```go
// Validate connector configuration at registration
func ValidateConfig(connectorType string, config interface{}) error {
    switch connectorType {
    case "myissuer":
        cfg := config.(*MyConfig)
        if cfg.Host == "" {
            return fmt.Errorf("host is required")
        }
        if cfg.Token == "" {
            return fmt.Errorf("token is required")
        }
    }
    return nil
}
```

### 5. Use in Your Application

```go
// Get connector
connector, err := issuer.GetConnector("myissuer", config)

// Issue certificate
resp, err := connector.IssueCertificate(ctx, issueReq)
```

---

## Best Practices

1. **Error Handling** — Return descriptive errors with context
2. **Timeout Management** — Always use context with timeouts
3. **Validation** — Validate configuration during Validate()
4. **Retry Logic** — Handle transient failures gracefully
5. **Logging** — Log all operations for debugging
6. **Testing** — Provide unit and integration tests
7. **Documentation** — Document configuration options and limitations
8. **Security** — Never log sensitive data (tokens, keys, passwords)

---

## Contributing Connectors

To contribute a connector to certctl:

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-connector`
3. Add connector implementation with tests
4. Update [README.md](../README.md#supported-integrations)
5. Add documentation to [docs/](.)
6. Submit a pull request

Connectors must:
- Implement the full interface
- Include unit tests (>80% coverage)
- Have integration tests (if applicable)
- Include configuration examples
- Document any prerequisites (API keys, credentials)

---

For more information, see:
- [Architecture Guide](architecture.md#connector-architecture)
- [API Reference](../README.md#api-overview)
- [Contributing Guidelines](../CONTRIBUTING.md) (coming soon)
