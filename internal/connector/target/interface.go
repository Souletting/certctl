package target

import (
	"context"
	"encoding/json"
	"time"
)

// Connector defines the interface for certificate deployment operations.
type Connector interface {
	// ValidateConfig validates the deployment target configuration.
	ValidateConfig(ctx context.Context, config json.RawMessage) error

	// DeployCertificate deploys a certificate to the target.
	// The request contains the certificate and chain in PEM format, but never a private key.
	DeployCertificate(ctx context.Context, request DeploymentRequest) (*DeploymentResult, error)

	// ValidateDeployment verifies that a deployed certificate is valid and accessible.
	ValidateDeployment(ctx context.Context, request ValidationRequest) (*ValidationResult, error)
}

// DeploymentRequest contains the parameters for deploying a certificate to a target.
// Note: This request NEVER contains a private key. The agent generates keys locally.
type DeploymentRequest struct {
	CertPEM       string          `json:"cert_pem"`
	ChainPEM      string          `json:"chain_pem"`
	TargetConfig  json.RawMessage `json:"target_config"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// DeploymentResult contains the result of a successful certificate deployment.
type DeploymentResult struct {
	Success       bool      `json:"success"`
	TargetAddress string    `json:"target_address"`
	DeploymentID  string    `json:"deployment_id"`
	Message       string    `json:"message"`
	DeployedAt    time.Time `json:"deployed_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// ValidationRequest contains the parameters for validating a deployed certificate.
type ValidationRequest struct {
	CertificateID string          `json:"certificate_id"`
	Serial        string          `json:"serial"`
	TargetConfig  json.RawMessage `json:"target_config"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// ValidationResult contains the result of a certificate validation check.
type ValidationResult struct {
	Valid         bool      `json:"valid"`
	Serial        string    `json:"serial"`
	TargetAddress string    `json:"target_address"`
	Message       string    `json:"message"`
	ValidatedAt   time.Time `json:"validated_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}
