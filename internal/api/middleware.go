package api

import (
"encoding/json"
"net/http"
)

// AuthMiddleware validates the API key from the X-API-Key header
func AuthMiddleware(apiSecret string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		
		if apiKey == "" {
			writeJSONError(w, "missing API key", http.StatusUnauthorized)
			return
		}
		
		if apiKey != apiSecret {
			writeJSONError(w, "invalid API key", http.StatusUnauthorized)
			return
		}
		
		next(w, r)
	}
}

// writeJSONError writes a JSON error response
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
"error": message,
"code":  statusCode,
})
}
