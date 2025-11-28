package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		apiSecret      string
		headerValue    string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "valid API key",
			apiSecret:      "test-secret",
			headerValue:    "test-secret",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing API key",
			apiSecret:      "test-secret",
			headerValue:    "",
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "missing API key",
		},
		{
			name:           "invalid API key",
			apiSecret:      "test-secret",
			headerValue:    "wrong-secret",
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "invalid API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false
			next := func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			}

			handler := AuthMiddleware(tt.apiSecret, next)

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.headerValue != "" {
				req.Header.Set("X-API-Key", tt.headerValue)
			}
			rr := httptest.NewRecorder()

			handler(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				_ = json.Unmarshal(rr.Body.Bytes(), &response)
				assert.Equal(t, tt.expectedError, response["error"])
				assert.False(t, nextCalled)
			} else {
				assert.True(t, nextCalled)
			}
		})
	}
}

func TestWriteJSONError(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		statusCode int
	}{
		{
			name:       "unauthorized error",
			message:    "unauthorized",
			statusCode: http.StatusUnauthorized,
		},
		{
			name:       "internal server error",
			message:    "something went wrong",
			statusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeJSONError(rr, tt.message, tt.statusCode)

			assert.Equal(t, tt.statusCode, rr.Code)
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

			var response map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, tt.message, response["error"])
			assert.Equal(t, float64(tt.statusCode), response["code"])
		})
	}
}
