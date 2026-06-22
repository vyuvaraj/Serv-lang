package runtime

import (
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
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
			switch val := v.(type) {
			case string:
				req.Params["auth_"+k] = val
			case []interface{}:
				var strVals []string
				for _, item := range val {
					strVals = append(strVals, fmt.Sprint(item))
				}
				req.Params["auth_"+k] = strings.Join(strVals, ",")
			default:
				req.Params["auth_"+k] = fmt.Sprint(val)
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

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("failed to decode header")
	}
	var header struct {
		Kid string `json:"kid"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("failed to parse header JSON")
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

		// Fetch public key for kid
		pubKey, err := getOidcAesPublicKey(expectedIssuer, header.Kid)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve OIDC public key: %w", err)
		}

		// Verify RS256 signature
		if header.Alg != "RS256" {
			return nil, fmt.Errorf("unsupported token signing algorithm: %s", header.Alg)
		}

		signingInput := parts[0] + "." + parts[1]
		hashed := sha256.Sum256([]byte(signingInput))
		sig, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			return nil, errors.New("invalid signature encoding")
		}

		err = rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], sig)
		if err != nil {
			return nil, errors.New("invalid token signature")
		}
	}

	return claims, nil
}

type oidcConfig struct {
	JwksURI string `json:"jwks_uri"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

var (
	jwksCacheMu     sync.Mutex
	jwksCacheKeys   = make(map[string]*rsa.PublicKey)
	jwksCacheExp    time.Time
	jwksCacheIssuer string
	jwksCacheURI    string
)

func getOidcAesPublicKey(issuer, kid string) (*rsa.PublicKey, error) {
	jwksCacheMu.Lock()
	defer jwksCacheMu.Unlock()

	// If issuer changed, clear cache
	if jwksCacheIssuer != issuer {
		jwksCacheKeys = make(map[string]*rsa.PublicKey)
		jwksCacheIssuer = issuer
		jwksCacheURI = ""
		jwksCacheExp = time.Time{}
	}

	// Return from cache if valid and kid exists
	if time.Now().Before(jwksCacheExp) {
		if pubKey, exists := jwksCacheKeys[kid]; exists {
			return pubKey, nil
		}
	}

	// Fetch discovery if URI is not resolved
	if jwksCacheURI == "" {
		wellKnownURL := issuer
		if !strings.HasPrefix(wellKnownURL, "http://") && !strings.HasPrefix(wellKnownURL, "https://") {
			if strings.HasPrefix(wellKnownURL, "localhost") || strings.HasPrefix(wellKnownURL, "127.0.0.1") {
				wellKnownURL = "http://" + wellKnownURL
			} else {
				wellKnownURL = "https://" + wellKnownURL
			}
		}
		wellKnownURL = strings.TrimSuffix(wellKnownURL, "/") + "/.well-known/openid-configuration"

		resp, err := http.Get(wellKnownURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch OIDC discovery: %w", err)
		}
		defer resp.Body.Close()

		var config oidcConfig
		if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
			return nil, fmt.Errorf("failed to decode OIDC discovery JSON: %w", err)
		}
		jwksCacheURI = config.JwksURI
	}

	// Fetch JWKS keys
	resp, err := http.Get(jwksCacheURI)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS keys: %w", err)
	}
	defer resp.Body.Close()

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode JWKS response: %w", err)
	}

	newKeys := make(map[string]*rsa.PublicKey)
	for _, key := range jwks.Keys {
		if key.Kty == "RSA" && key.N != "" && key.E != "" {
			pubKey, err := parseRSAPublicKey(key.N, key.E)
			if err == nil {
				newKeys[key.Kid] = pubKey
			}
		}
	}

	jwksCacheKeys = newKeys
	jwksCacheExp = time.Now().Add(1 * time.Hour)

	pubKey, exists := jwksCacheKeys[kid]
	if !exists {
		return nil, fmt.Errorf("key ID %s not found in JWKS", kid)
	}
	return pubKey, nil
}

func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	decN, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, err
	}
	decE, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, err
	}
	var eVal int
	for _, b := range decE {
		eVal = (eVal << 8) | int(b)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(decN),
		E: eVal,
	}, nil
}
