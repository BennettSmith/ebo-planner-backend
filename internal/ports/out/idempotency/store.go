package idempotency

import (
	"context"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
)

// Key is the caller-provided idempotency key (Idempotency-Key header).
type Key string

// Fingerprint identifies a request uniquely for idempotency purposes.
//
// v1 strategy (per implementation plan) is: key + route + subject + request body hash.
// Route is represented as HTTP method + normalized path template (e.g. "PUT /trips/{tripId}/rsvp").
type Fingerprint struct {
	Key      Key
	Subject  domain.SubjectID
	Method   string
	Route    string
	BodyHash string
}

// Record is the stored response we can replay for a duplicate request.
type Record struct {
	StatusCode  int
	ContentType string
	Body        []byte
	CreatedAt   time.Time
}

// Store persists idempotency records for replaying safe responses on retries.
type Store interface {
	Get(ctx context.Context, fp Fingerprint) (Record, bool, error)
	Put(ctx context.Context, fp Fingerprint, rec Record) error
}
