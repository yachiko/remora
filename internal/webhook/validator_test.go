package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidator_ValidateSignature(t *testing.T) {
	tests := []struct {
		name      string
		secret    string
		payload   []byte
		signature string
		wantErr   bool
	}{
		{
			name:      "valid signature",
			secret:    "my-secret",
			payload:   []byte("test payload"),
			signature: "sha256=fbf75167cce528499b48e9041e3aab3188281962af6b9f19a9b43e7bbb08fc64",
			wantErr:   false,
		},
		{
			name:      "invalid signature",
			secret:    "my-secret",
			payload:   []byte("test payload"),
			signature: "sha256=invalidhash",
			wantErr:   true,
		},
		{
			name:      "missing signature",
			secret:    "my-secret",
			payload:   []byte("test payload"),
			signature: "",
			wantErr:   true,
		},
		{
			name:      "wrong prefix",
			secret:    "my-secret",
			payload:   []byte("test payload"),
			signature: "sha1=somehash",
			wantErr:   true,
		},
		{
			name:      "wrong secret",
			secret:    "wrong-secret",
			payload:   []byte("test payload"),
			signature: "sha256=fbf75167cce528499b48e9041e3aab3188281962af6b9f19a9b43e7bbb08fc64",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(tt.secret)
			err := v.ValidateSignature(tt.payload, tt.signature)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ReadAndValidatePayload(t *testing.T) {
	secret := "test-secret"
	v := NewValidator(secret)

	t.Run("valid payload", func(t *testing.T) {
		payload := []byte(`{"test": "data"}`)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
		req.Header.Set(SignatureHeader, signature)

		result, err := v.ReadAndValidatePayload(req)
		assert.NoError(t, err)
		assert.Equal(t, payload, result)
	})

	t.Run("invalid signature", func(t *testing.T) {
		payload := []byte(`{"test": "data"}`)
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
		req.Header.Set(SignatureHeader, "sha256=invalid")

		_, err := v.ReadAndValidatePayload(req)
		assert.Error(t, err)
	})

	t.Run("payload too large", func(t *testing.T) {
		// Create a payload larger than MaxPayloadSize
		payload := make([]byte, MaxPayloadSize+1)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
		req.Header.Set(SignatureHeader, signature)

		_, err := v.ReadAndValidatePayload(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "payload too large")
	})
}

func TestIssueCommentEvent_Validate(t *testing.T) {
	validEvent := IssueCommentEvent{
		Action: "created",
		Comment: Comment{
			ID:      123,
			Body:    "test",
			HTMLURL: "https://github.com/owner/repo/issues/1#issuecomment-123",
			User:    User{ID: 456, Login: "user"},
		},
		Issue:        Issue{Number: 1},
		Repository:   Repository{Name: "repo", FullName: "owner/repo", Owner: User{Login: "owner"}},
		Installation: Installation{ID: 789},
	}

	assert.NoError(t, validEvent.Validate())

	// Test missing fields
	tests := []struct {
		name    string
		modify  func(*IssueCommentEvent)
		wantErr bool
	}{
		{"missing action", func(e *IssueCommentEvent) { e.Action = "" }, true},
		{"missing comment.id", func(e *IssueCommentEvent) { e.Comment.ID = 0 }, true},
		{"missing comment.body", func(e *IssueCommentEvent) { e.Comment.Body = "" }, true},
		{"missing comment.user.login", func(e *IssueCommentEvent) { e.Comment.User.Login = "" }, true},
		{"missing issue.number", func(e *IssueCommentEvent) { e.Issue.Number = 0 }, true},
		{"missing repository.name", func(e *IssueCommentEvent) { e.Repository.Name = "" }, true},
		{"missing installation.id", func(e *IssueCommentEvent) { e.Installation.ID = 0 }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent
			tt.modify(&event)
			err := event.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetEventType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Header.Set(EventTypeHeader, "issue_comment")

	eventType := GetEventType(req)
	assert.Equal(t, "issue_comment", eventType)
}

func TestGetDeliveryID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Header.Set(DeliveryHeader, "delivery-123")

	deliveryID := GetDeliveryID(req)
	assert.Equal(t, "delivery-123", deliveryID)
}
