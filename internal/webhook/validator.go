package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// MaxPayloadSize is the maximum allowed webhook payload size (1 MB)
	MaxPayloadSize = 1024 * 1024

	// SignatureHeader is the HTTP header containing the webhook signature
	SignatureHeader = "X-Hub-Signature-256"

	// EventTypeHeader is the HTTP header containing the event type
	EventTypeHeader = "X-GitHub-Event"

	// DeliveryHeader is the HTTP header containing the delivery ID
	DeliveryHeader = "X-GitHub-Delivery"
)

// Validator handles webhook signature validation
type Validator struct {
	secret string
}

// NewValidator creates a new webhook validator
func NewValidator(secret string) *Validator {
	return &Validator{
		secret: secret,
	}
}

// ValidateSignature validates the GitHub webhook signature
// It computes HMAC-SHA256 of the payload and compares with the provided signature
func (v *Validator) ValidateSignature(payload []byte, signature string) error {
	if signature == "" {
		return fmt.Errorf("missing signature header")
	}

	// Signature format: "sha256=<hex-encoded-hash>"
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format: expected 'sha256=' prefix")
	}

	// Extract the hex hash from signature
	providedHash := strings.TrimPrefix(signature, "sha256=")

	// Compute HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(v.secret))
	mac.Write(payload)
	expectedHash := hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison to prevent timing attacks
	if !hmac.Equal([]byte(expectedHash), []byte(providedHash)) {
		return fmt.Errorf("signature mismatch: invalid webhook signature")
	}

	return nil
}

// ReadAndValidatePayload reads the request body and validates the signature
// Returns the payload bytes or an error
func (v *Validator) ReadAndValidatePayload(r *http.Request) ([]byte, error) {
	// Check content length
	if r.ContentLength > MaxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d bytes (max: %d)", r.ContentLength, MaxPayloadSize)
	}

	// Read the body with size limit
	limitedReader := io.LimitReader(r.Body, MaxPayloadSize+1)
	payload, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// Check if payload exceeded limit
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("payload too large: exceeds %d bytes", MaxPayloadSize)
	}

	// Get signature from header
	signature := r.Header.Get(SignatureHeader)

	// Validate signature
	if err := v.ValidateSignature(payload, signature); err != nil {
		return nil, err
	}

	return payload, nil
}

// GetEventType extracts the event type from the request headers
func GetEventType(r *http.Request) string {
	return r.Header.Get(EventTypeHeader)
}

// GetDeliveryID extracts the delivery ID from the request headers
func GetDeliveryID(r *http.Request) string {
	return r.Header.Get(DeliveryHeader)
}
