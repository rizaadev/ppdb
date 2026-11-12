package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrStorageNotConfigured indicates missing object storage endpoint or credentials.
	ErrStorageNotConfigured = errors.New("object storage is not configured")
	// ErrInvalidObjectKey indicates a storage key that violates tenant/application scope rules.
	ErrInvalidObjectKey = errors.New("invalid object storage key")
	// ErrObjectNotFound indicates the referenced object does not exist in storage yet.
	ErrObjectNotFound = errors.New("uploaded object not found in storage")
)

// PresignedResult holds a short-lived presigned URL and required upload headers.
type PresignedResult struct {
	URL             string            `json:"url"`
	StorageKey      string            `json:"storage_key"`
	RequiredHeaders map[string]string `json:"required_headers,omitempty"`
	ExpiresAt       time.Time         `json:"expires_at"`
}

// ObjectMetadata captures authoritative object attributes read back from storage.
type ObjectMetadata struct {
	SizeBytes   int64
	ContentType string
}

// ObjectStorage abstracts private document, proof, logo, and export file persistence across S3/MinIO/R2 and local signed disk storage.
type ObjectStorage interface {
	PresignPutObject(key, mimeType string, ttl time.Duration) (PresignedResult, error)
	PresignGetObject(key string, ttl time.Duration) (PresignedResult, error)
	PutObject(ctx context.Context, key, mimeType string, body []byte) error
	GetObject(ctx context.Context, key string) ([]byte, string, error)
	HeadObject(ctx context.Context, key string) (*ObjectMetadata, error)
	DeleteObject(ctx context.Context, key string) error
}

// BuildManualProofKey builds a tenant- and payment-scoped object key for manual bank transfer receipts.
func BuildManualProofKey(tenantID, applicationID, paymentID uuid.UUID, filename string) string {
	id := uuid.Must(uuid.NewV7())
	return fmt.Sprintf("%s/%s/payments/%s/%s-%s", tenantID.String(), applicationID.String(), paymentID.String(), id.String(), SanitizeFilename(filename))
}

// ValidateManualProofKey ensures proofKey remains strictly within the given tenant, application, and payment scope.
func ValidateManualProofKey(proofKey string, tenantID, applicationID, paymentID uuid.UUID) bool {
	if strings.Contains(proofKey, "..") || strings.Contains(proofKey, `\`) {
		return false
	}
	expectedPrefix := fmt.Sprintf("%s/%s/payments/%s/", tenantID.String(), applicationID.String(), paymentID.String())
	if !strings.HasPrefix(proofKey, expectedPrefix) {
		return false
	}
	rest := strings.TrimPrefix(proofKey, expectedPrefix)
	return rest != "" && !strings.Contains(rest, "/")
}

// BuildTenantLogoKey builds a tenant-scoped object key for school branding logos.
func BuildTenantLogoKey(tenantID uuid.UUID, filename string) string {
	id := uuid.Must(uuid.NewV7())
	return fmt.Sprintf("%s/branding/%s-%s", tenantID.String(), id.String(), SanitizeFilename(filename))
}

// ValidateTenantLogoKey ensures a tenant logo storage key stays strictly inside the tenant's branding folder.
func ValidateTenantLogoKey(logoKey string, tenantID uuid.UUID) bool {
	if strings.Contains(logoKey, "..") || strings.Contains(logoKey, `\`) {
		return false
	}
	expectedPrefix := fmt.Sprintf("%s/branding/", tenantID.String())
	if !strings.HasPrefix(logoKey, expectedPrefix) {
		return false
	}
	rest := strings.TrimPrefix(logoKey, expectedPrefix)
	return rest != "" && !strings.Contains(rest, "/")
}

