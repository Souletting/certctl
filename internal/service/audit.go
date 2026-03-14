package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shankar0123/certctl/internal/domain"
	"github.com/shankar0123/certctl/internal/repository"
)

// AuditService provides business logic for recording and retrieving audit events.
type AuditService struct {
	auditRepo repository.AuditRepository
}

// NewAuditService creates a new audit service.
func NewAuditService(auditRepo repository.AuditRepository) *AuditService {
	return &AuditService{
		auditRepo: auditRepo,
	}
}

// RecordEvent records an audit event with actor, action, and resource information.
func (s *AuditService) RecordEvent(ctx context.Context, actor string, actorType domain.ActorType, action string, resourceType string, resourceID string, details map[string]interface{}) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		detailsJSON = []byte("{}")
	}

	event := &domain.AuditEvent{
		ID:           generateID("audit"),
		Timestamp:    time.Now(),
		Actor:        actor,
		ActorType:    actorType,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      json.RawMessage(detailsJSON),
	}

	if err := s.auditRepo.Create(ctx, event); err != nil {
		return fmt.Errorf("failed to record audit event: %w", err)
	}

	return nil
}

// List returns audit events matching filter criteria.
func (s *AuditService) List(ctx context.Context, filter *repository.AuditFilter) ([]*domain.AuditEvent, error) {
	events, err := s.auditRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events: %w", err)
	}
	return events, nil
}

// ListByResource returns all audit events for a specific resource.
func (s *AuditService) ListByResource(ctx context.Context, resourceType string, resourceID string) ([]*domain.AuditEvent, error) {
	filter := &repository.AuditFilter{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		PerPage:      1000, // reasonable default for single resource
	}

	events, err := s.auditRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events: %w", err)
	}
	return events, nil
}

// ListByActor returns all audit events for a specific actor.
func (s *AuditService) ListByActor(ctx context.Context, actor string) ([]*domain.AuditEvent, error) {
	filter := &repository.AuditFilter{
		Actor:   actor,
		PerPage: 1000,
	}

	events, err := s.auditRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events: %w", err)
	}
	return events, nil
}

// ListByAction returns all audit events for a specific action type.
func (s *AuditService) ListByAction(ctx context.Context, action string, from, to time.Time) ([]*domain.AuditEvent, error) {
	filter := &repository.AuditFilter{
		From:    from,
		To:      to,
		PerPage: 1000,
	}

	events, err := s.auditRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events: %w", err)
	}

	// Filter by action on client side (repository may not filter by action directly)
	var filtered []*domain.AuditEvent
	for _, e := range events {
		if e.Action == action {
			filtered = append(filtered, e)
		}
	}

	return filtered, nil
}
