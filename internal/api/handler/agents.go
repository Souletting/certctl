package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/shankar0123/certctl/internal/api/middleware"
	"github.com/shankar0123/certctl/internal/domain"
)

// AgentService defines the service interface for agent operations.
type AgentService interface {
	ListAgents(page, perPage int) ([]domain.Agent, int64, error)
	GetAgent(id string) (*domain.Agent, error)
	RegisterAgent(agent domain.Agent) (*domain.Agent, error)
	Heartbeat(agentID string) error
	CSRSubmit(agentID string, csrPEM string) (string, error)
	CertificatePickup(agentID, certID string) (string, error)
}

// AgentHandler handles HTTP requests for agent operations.
type AgentHandler struct {
	svc AgentService
}

// NewAgentHandler creates a new AgentHandler with a service dependency.
func NewAgentHandler(svc AgentService) AgentHandler {
	return AgentHandler{svc: svc}
}

// ListAgents lists all registered agents.
// GET /api/v1/agents?page=1&per_page=50
func (h AgentHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	requestID := middleware.GetRequestID(r.Context())

	page := 1
	perPage := 50
	query := r.URL.Query()
	if p := query.Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if pp := query.Get("per_page"); pp != "" {
		if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 500 {
			perPage = parsed
		}
	}

	agents, total, err := h.svc.ListAgents(page, perPage)
	if err != nil {
		ErrorWithRequestID(w, http.StatusInternalServerError, "Failed to list agents", requestID)
		return
	}

	response := PagedResponse{
		Data:    agents,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	}

	JSON(w, http.StatusOK, response)
}

// GetAgent retrieves a single agent by ID.
// GET /api/v1/agents/{id}
func (h AgentHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	requestID := middleware.GetRequestID(r.Context())

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(id, "/")
	if len(parts) == 0 || parts[0] == "" {
		ErrorWithRequestID(w, http.StatusBadRequest, "Agent ID is required", requestID)
		return
	}
	id = parts[0]

	agent, err := h.svc.GetAgent(id)
	if err != nil {
		ErrorWithRequestID(w, http.StatusNotFound, "Agent not found", requestID)
		return
	}

	JSON(w, http.StatusOK, agent)
}

// RegisterAgent registers a new agent.
// POST /api/v1/agents
func (h AgentHandler) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	requestID := middleware.GetRequestID(r.Context())

	var agent domain.Agent
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		ErrorWithRequestID(w, http.StatusBadRequest, "Invalid request body", requestID)
		return
	}

	created, err := h.svc.RegisterAgent(agent)
	if err != nil {
		ErrorWithRequestID(w, http.StatusInternalServerError, "Failed to register agent", requestID)
		return
	}

	JSON(w, http.StatusCreated, created)
}

// Heartbeat records a heartbeat from an agent.
// POST /api/v1/agents/{id}/heartbeat
func (h AgentHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	requestID := middleware.GetRequestID(r.Context())

	// Extract agent ID from path /api/v1/agents/{id}/heartbeat
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" {
		ErrorWithRequestID(w, http.StatusBadRequest, "Agent ID is required", requestID)
		return
	}
	agentID := parts[0]

	if err := h.svc.Heartbeat(agentID); err != nil {
		ErrorWithRequestID(w, http.StatusInternalServerError, "Failed to record heartbeat", requestID)
		return
	}

	response := map[string]string{
		"status": "heartbeat_recorded",
	}

	JSON(w, http.StatusOK, response)
}

// AgentCSRSubmit receives a Certificate Signing Request from an agent.
// POST /api/v1/agents/{id}/csr
func (h AgentHandler) AgentCSRSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	requestID := middleware.GetRequestID(r.Context())

	// Extract agent ID from path /api/v1/agents/{id}/csr
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" {
		ErrorWithRequestID(w, http.StatusBadRequest, "Agent ID is required", requestID)
		return
	}
	agentID := parts[0]

	var req struct {
		CSRPEM string `json:"csr_pem"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorWithRequestID(w, http.StatusBadRequest, "Invalid request body", requestID)
		return
	}

	if req.CSRPEM == "" {
		ErrorWithRequestID(w, http.StatusBadRequest, "CSR PEM is required", requestID)
		return
	}

	jobID, err := h.svc.CSRSubmit(agentID, req.CSRPEM)
	if err != nil {
		ErrorWithRequestID(w, http.StatusInternalServerError, "Failed to submit CSR", requestID)
		return
	}

	response := map[string]string{
		"job_id": jobID,
		"status": "csr_received",
	}

	JSON(w, http.StatusAccepted, response)
}

// AgentCertificatePickup allows an agent to retrieve an issued certificate.
// GET /api/v1/agents/{id}/certificates/{cert_id}
func (h AgentHandler) AgentCertificatePickup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	requestID := middleware.GetRequestID(r.Context())

	// Extract agent ID and certificate ID from path /api/v1/agents/{id}/certificates/{cert_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[0] == "" || parts[2] == "" {
		ErrorWithRequestID(w, http.StatusBadRequest, "Agent ID and Certificate ID are required", requestID)
		return
	}
	agentID := parts[0]
	certID := parts[2]

	certPEM, err := h.svc.CertificatePickup(agentID, certID)
	if err != nil {
		ErrorWithRequestID(w, http.StatusNotFound, "Certificate not found or not ready", requestID)
		return
	}

	response := map[string]string{
		"certificate_pem": certPEM,
	}

	JSON(w, http.StatusOK, response)
}
