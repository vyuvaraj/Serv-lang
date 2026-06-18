package runtime

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	authSecretOrProvider string
)

func InitAuth(secretOrProvider string) {
	authSecretOrProvider = secretOrProvider
	RegisterMiddleware("auth", func(req Request) interface{} {
		authHeader := req.Headers["authorization"]
		if authHeader == "" {
			authHeader = req.Headers["Authorization"]
		}
		if authHeader == "" {
			return map[string]interface{}{
				"status": 401,
				"error":  "Unauthorized",
				"code":   "ERR_UNAUTHORIZED",
				"message": "Missing Authorization header",
			}
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return map[string]interface{}{
				"status": 401,
				"error":  "Unauthorized",
				"code":   "ERR_UNAUTHORIZED",
				"message": "Invalid Authorization header format. Expected 'Bearer <token>'",
			}
		}

		token := parts[1]
		claims, err := VerifyToken(token, authSecretOrProvider)
		if err != nil {
			return map[string]interface{}{
				"status": 401,
				"error":  "Unauthorized",
				"code":   "ERR_UNAUTHORIZED",
				"message": err.Error(),
			}
		}

		// Inject claims into request params or context so handlers can access them
		for k, v := range claims {
			if strVal, ok := v.(string); ok {
				req.Params["auth_"+k] = strVal
			}
		}

		return nil // validation passed, continue to next middleware/handler
	})
}

// VerifyToken decodes and validates a JWT token against the configured secret/issuer
func VerifyToken(token, secretOrProvider string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT token format")
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("failed to decode claims")
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, errors.New("failed to parse claims JSON")
	}

	// Expiration check
	if expVal, exists := claims["exp"]; exists {
		var expTime time.Time
		switch v := expVal.(type) {
		case float64:
			expTime = time.Unix(int64(v), 0)
		case int64:
			expTime = time.Unix(v, 0)
		}
		if !expTime.IsZero() && time.Now().After(expTime) {
			return nil, errors.New("token has expired")
		}
	}

	// Validate signature if it's jwt://
	if strings.HasPrefix(secretOrProvider, "jwt://") {
		secret := strings.TrimPrefix(secretOrProvider, "jwt://")
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(parts[0] + "." + parts[1]))
		expectedSig := mac.Sum(nil)

		sig, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			return nil, errors.New("invalid signature encoding")
		}

		if !hmac.Equal(sig, expectedSig) {
			return nil, errors.New("invalid token signature")
		}
	} else if strings.HasPrefix(secretOrProvider, "oidc://") {
		// For OIDC, check issuer matches
		expectedIssuer := strings.TrimPrefix(secretOrProvider, "oidc://")
		if iss, exists := claims["iss"]; exists {
			if issStr, ok := iss.(string); ok && !strings.Contains(issStr, expectedIssuer) {
				return nil, errors.New("token issuer mismatch")
			}
		}
	}

	return claims, nil
}
