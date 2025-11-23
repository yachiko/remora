package github

import (
	"context"
	"crypto/rsa"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// Client handles GitHub App authentication and API interactions
type Client struct {
	appID       int64
	privateKey  *rsa.PrivateKey
	httpClient  *http.Client
	logger      *zap.Logger
	tokenCache  *tokenCache
	baseURL     string
	rateLimiter *rateLimiter
	retryConfig *RetryConfig
}

// tokenCache stores installation tokens with expiration
type tokenCache struct {
	tokens map[int64]*cachedToken
	mu     sync.RWMutex
}

// cachedToken represents a cached installation token
type cachedToken struct {
	Token     string
	ExpiresAt time.Time
}

// NewClient creates a new GitHub client
func NewClient(appID int64, privateKey *rsa.PrivateKey, logger *zap.Logger) *Client {
	return &Client{
		appID:      appID,
		privateKey: privateKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
		tokenCache: &tokenCache{
			tokens: make(map[int64]*cachedToken),
		},
		baseURL: "https://api.github.com",
		rateLimiter: &rateLimiter{
			remaining: 5000,
			resetAt:   time.Now(),
		},
		retryConfig: DefaultRetryConfig(),
	}
}

// generateJWT creates a JWT for GitHub App authentication
// JWTs are valid for 10 minutes and used to obtain installation tokens
func (c *Client) generateJWT() (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		Issuer:    fmt.Sprintf("%d", c.appID),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	signedToken, err := token.SignedString(c.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return signedToken, nil
}

// GetInstallationToken retrieves or refreshes an installation token
// Tokens are cached for 55 minutes (5-minute safety buffer before 60-minute expiration)
func (c *Client) GetInstallationToken(ctx context.Context, installationID int64) (string, error) {
	// Check cache first
	c.tokenCache.mu.RLock()
	cached, exists := c.tokenCache.tokens[installationID]
	c.tokenCache.mu.RUnlock()

	// Return cached token if valid (with 5-minute safety buffer)
	if exists && time.Now().Before(cached.ExpiresAt.Add(-5*time.Minute)) {
		c.logger.Debug("using cached installation token",
			zap.Int64("installation_id", installationID),
			zap.Time("expires_at", cached.ExpiresAt))
		return cached.Token, nil
	}

	// Token expired or doesn't exist, fetch new one
	return c.refreshInstallationToken(ctx, installationID)
}

// refreshInstallationToken fetches a new installation token from GitHub
func (c *Client) refreshInstallationToken(ctx context.Context, installationID int64) (string, error) {
	c.tokenCache.mu.Lock()
	defer c.tokenCache.mu.Unlock()

	// Double-check pattern: another goroutine may have refreshed while we waited
	if cached, exists := c.tokenCache.tokens[installationID]; exists {
		if time.Now().Before(cached.ExpiresAt.Add(-5 * time.Minute)) {
			return cached.Token, nil
		}
	}

	c.logger.Info("refreshing installation token",
		zap.Int64("installation_id", installationID))

	// Generate JWT for authentication
	jwtToken, err := c.generateJWT()
	if err != nil {
		return "", fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Create request
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", c.baseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	// Execute request with retry logic
	var response struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	err = c.doRequestWithRetry(ctx, req, &response)
	if err != nil {
		return "", fmt.Errorf("failed to get installation token: %w", err)
	}

	// Cache the new token
	c.tokenCache.tokens[installationID] = &cachedToken{
		Token:     response.Token,
		ExpiresAt: response.ExpiresAt,
	}

	c.logger.Info("installation token refreshed",
		zap.Int64("installation_id", installationID),
		zap.Time("expires_at", response.ExpiresAt))

	return response.Token, nil
}

// InvalidateToken removes a token from the cache (useful after 401 errors)
func (c *Client) InvalidateToken(installationID int64) {
	c.tokenCache.mu.Lock()
	defer c.tokenCache.mu.Unlock()

	delete(c.tokenCache.tokens, installationID)

	c.logger.Info("invalidated cached token",
		zap.Int64("installation_id", installationID))
}
