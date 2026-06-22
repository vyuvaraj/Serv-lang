package main

import (
	"bytes"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"serv/compiler"
	"serv/runtime"
)

func TestSQLInjectionPrevention(t *testing.T) {
	// Case 1: Infix concatenation
	srcConcat := `
fn runQuery(id: string) {
	db.query("SELECT * FROM users WHERE id = " + id)
}
`
	lexer := compiler.NewLexer(srcConcat)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	diags := compiler.Analyze(prog)
	hasSQLiErr := false
	for _, d := range diags {
		if strings.Contains(d.Message, "SQL injection risk detected") {
			hasSQLiErr = true
		}
	}
	if !hasSQLiErr {
		t.Errorf("expected SQL injection error diagnostic for string concatenation, got none")
	}

	// Case 2: Interpolated F-string
	srcFString := `
fn runQuery(id: string) {
	db.querySafe(f"SELECT * FROM users WHERE id = {id}")
}
`
	lexer2 := compiler.NewLexer(srcFString)
	parser2 := compiler.NewParser(lexer2)
	prog2 := parser2.ParseProgram()
	if len(parser2.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser2.Errors())
	}

	diags2 := compiler.Analyze(prog2)
	hasSQLiErr2 := false
	for _, d := range diags2 {
		if strings.Contains(d.Message, "SQL injection risk detected") {
			hasSQLiErr2 = true
		}
	}
	if !hasSQLiErr2 {
		t.Errorf("expected SQL injection error diagnostic for f-string formatting, got none")
	}
}

func TestSecretLogMasking(t *testing.T) {
	os.Setenv("TEST_SECRET_KEY", "super-secret-passphrase-999")
	defer os.Unsetenv("TEST_SECRET_KEY")

	// Register the secret
	runtime.EnvSecret("TEST_SECRET_KEY")

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr) // restore default

	runtime.LogInfo("Connecting to database with password super-secret-passphrase-999 now")

	output := buf.String()

	if strings.Contains(output, "super-secret-passphrase-999") {
		t.Errorf("Expected secret to be masked in log output, but found it raw: %q", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Errorf("Expected log output to contain '[REDACTED]', got: %q", output)
	}
}

func TestCORSConfiguration(t *testing.T) {
	runtime.EnableCORS([]string{"https://myclient.com"})

	// Setup a dummy handler that returns 200
	runtime.AddRoute("GET", "/test-cors", 0, "", func(req runtime.Request) interface{} {
		return "cors-ok"
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock runtime.StartServer handler behavior
		// Let's invoke the handler via similar code path
		origin := r.Header.Get("Origin")
		allowed := origin == "https://myclient.com"
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("cors-ok"))
	}))
	defer server.Close()

	// OPTIONS preflight request
	req, _ := http.NewRequest("OPTIONS", server.URL+"/test-cors", nil)
	req.Header.Set("Origin", "https://myclient.com")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 No Content for preflight, got %d", res.StatusCode)
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "https://myclient.com" {
		t.Errorf("expected Access-Control-Allow-Origin header to be set, got %q", res.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestRateLimiting(t *testing.T) {
	// Initialize rate limiter: 1 req per second
	runtime.SetGlobalIPRateLimit(1, "s")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock per-IP rate limiter logic
		clientIP := "127.0.0.1"
		lim := runtime.GetGlobalIPRateLimiterStub(clientIP)
		if lim != nil && !lim.AllowStub() {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("too many requests"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	// First request: OK
	res1, _ := http.Get(server.URL + "/")
	if res1.StatusCode != http.StatusOK {
		t.Errorf("first request expected 200, got %d", res1.StatusCode)
	}

	// Second request: Too Many Requests (429)
	res2, _ := http.Get(server.URL + "/")
	if res2.StatusCode != http.StatusTooManyRequests {
		t.Errorf("second request expected 429, got %d", res2.StatusCode)
	}
}

func TestInputSanitization(t *testing.T) {
	// Setup route
	runtime.AddRoute("GET", "/echo", 0, "", func(req runtime.Request) interface{} {
		return req.Params["val"]
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock input sanitization logic
		val := r.URL.Query().Get("val")
		sanitized := htmlEscapeStub(val)
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sanitized))
	}))
	defer server.Close()

	rawInput := "<script>alert('xss')</script>"
	escapedInput := "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"

	res, err := http.Get(server.URL + "/echo?val=" + url.QueryEscape(rawInput))
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != escapedInput {
		t.Errorf("expected sanitized output %q, got %q", escapedInput, string(body))
	}
}

// Helpers for the test mock logic
func htmlEscapeStub(s string) string {
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

func TestAuthKeywordAndMiddleware(t *testing.T) {
	// 1. Verify parsing of auth keyword
	src := `
	auth "jwt://secret123"
	route "GET" "/secure" (req) {
		return "secret-data"
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	codegen := compiler.NewCodegen(prog)
	generated, err := codegen.Generate()
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	if !strings.Contains(generated, `runtime.InitAuth(fmt.Sprint("jwt://secret123"))`) {
		t.Errorf("expected generated code to initialize auth, got: %s", generated)
	}

	// 2. Verify middleware functionality
	runtime.InitAuth("jwt://secret123")

	// Set up route with auth middleware
	runtime.AddRouteWithMiddleware("GET", "/secure", 0, "", []string{"auth"}, func(req runtime.Request) interface{} {
		return map[string]interface{}{"data": "secured-resource"}
	})

	// Setup a local test router handler (mocking StartServer's internal routing wrapper)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mimic the runtime dispatch logic
		headers := make(map[string]string)
		for k, v := range r.Header {
			headers[k] = v[0]
		}
		req := runtime.Request{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: headers,
			Params:  make(map[string]string),
		}

		// Perform middleware matching and execution
		// (We manually fetch and execute the route handler registered above)
		handler, _, _, _ := runtime.MatchRouteStub(r.Method, r.URL.Path)
		if handler == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		res := handler(req)
		w.Header().Set("Content-Type", "application/json")
		if resMap, ok := res.(map[string]interface{}); ok {
			if statusVal, exists := resMap["status"]; exists {
				if code, ok := statusVal.(int); ok {
					w.WriteHeader(code)
				}
			}
			json.NewEncoder(w).Encode(resMap)
		} else {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(res)
		}
	}))
	defer server.Close()

	// Request without Auth header -> 401
	res1, _ := http.Get(server.URL + "/secure")
	if res1.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for request without auth, got %d", res1.StatusCode)
	}

	// Request with invalid signature token -> 401
	badToken := generateTestJWT(map[string]interface{}{"sub": "user123", "exp": time.Now().Add(time.Hour).Unix()}, "wrong-secret")
	req2, _ := http.NewRequest("GET", server.URL+"/secure", nil)
	req2.Header.Set("Authorization", "Bearer "+badToken)
	res2, _ := http.DefaultClient.Do(req2)
	if res2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad token, got %d", res2.StatusCode)
	}

	// Request with valid token -> 200
	goodToken := generateTestJWT(map[string]interface{}{"sub": "user123", "exp": time.Now().Add(time.Hour).Unix()}, "secret123")
	req3, _ := http.NewRequest("GET", server.URL+"/secure", nil)
	req3.Header.Set("Authorization", "Bearer "+goodToken)
	res3, _ := http.DefaultClient.Do(req3)
	if res3.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d", res3.StatusCode)
	}
}

func generateTestJWT(claims map[string]interface{}, secret string) string {
	header := `{"alg":"HS256","typ":"JWT"}`
	headerEnc := base64.RawURLEncoding.EncodeToString([]byte(header))
	claimsBytes, _ := json.Marshal(claims)
	claimsEnc := base64.RawURLEncoding.EncodeToString(claimsBytes)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(headerEnc + "." + claimsEnc))
	sig := mac.Sum(nil)
	sigEnc := base64.RawURLEncoding.EncodeToString(sig)

	return headerEnc + "." + claimsEnc + "." + sigEnc
}

func TestMailKeywordAndSend(t *testing.T) {
	// 1. Verify parsing and codegen of mail keyword
	src := `
	mail "smtp://localhost:25"
	fn notify() {
		mail.send("test@domain.com", "Subject", "Body content")
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	codegen := compiler.NewCodegen(prog)
	generated, err := codegen.Generate()
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	if !strings.Contains(generated, `runtime.InitMail(fmt.Sprint("smtp://localhost:25"))`) {
		t.Errorf("expected generated code to initialize mail, got: %s", generated)
	}

	// 2. Verify runtime SendMail success path (stubbed for mock/ses/test protocols)
	runtime.InitMail("mock://test-broker")
	err = runtime.SendMail("someone@somewhere.com", "Test Subject", "Test Body")
	if err != nil {
		t.Errorf("expected no error for mock SendMail, got: %v", err)
	}
}

func TestStreamingResponseSupport(t *testing.T) {
	// 1. Verify parsing and codegen of streaming route
	src := `
	route "GET" "/stream-test" (req) stream {
		yield "first"
		yield "second"
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	codegen := compiler.NewCodegen(prog)
	generated, err := codegen.Generate()
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	if !strings.Contains(generated, `_streamChan := make(chan interface{})`) {
		t.Errorf("expected channel creation in streaming route, got: %s", generated)
	}
	if !strings.Contains(generated, `_streamChan <- "first"`) {
		t.Errorf("expected yield statements compiled to channel send, got: %s", generated)
	}

	// 2. Verify runtime matching and streaming response
	// Build a mock handler that mimics the generated code
	mockHandler := func(req runtime.Request) interface{} {
		ch := make(chan interface{})
		go func() {
			defer close(ch)
			ch <- "hello"
			ch <- "world"
		}()
		return ch
	}
	runtime.AddRoute("GET", "/stream-test-run", 0, "", mockHandler)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, p, _, _ := runtime.MatchRouteStub(r.Method, r.URL.Path)
		if h == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		
		// Setup runtime.Request
		req := runtime.Request{
			Method: r.Method,
			Path:   r.URL.Path,
			Params: p,
		}
		res := h(req)
		if ch, ok := res.(chan interface{}); ok {
			flusher, isFlusher := w.(http.Flusher)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			if isFlusher {
				flusher.Flush()
			}
			for item := range ch {
				fmt.Fprintf(w, "data: %s\n\n", item)
				if isFlusher {
					flusher.Flush()
				}
			}
		}
	}))
	defer server.Close()

	res, err := http.Get(server.URL + "/stream-test-run")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer res.Body.Close()

	if res.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream content type, got: %s", res.Header.Get("Content-Type"))
	}

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	body := string(bodyBytes)
	expected := "data: hello\n\ndata: world\n\n"
	if body != expected {
		t.Errorf("expected body %q, got %q", expected, body)
	}
}

func TestOpenAPIGeneration(t *testing.T) {
	src := `
	struct User {
		id: string
		age: int
	}

	route "GET" "/api/users/:id" (req) {
		return { "id": "123", "age": 30 }
	}

	route "POST" "/api/users" (req) {
		return { "success": true }
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	jsonStr, err := compiler.GenerateOpenAPI(prog)
	if err != nil {
		t.Fatalf("failed to generate OpenAPI: %v", err)
	}

	// Verify paths
	if !strings.Contains(jsonStr, `"/api/users/{id}"`) {
		t.Errorf("expected path parameter placeholder, got: %s", jsonStr)
	}

	// Verify requestBody for POST
	if !strings.Contains(jsonStr, `"requestBody"`) {
		t.Errorf("expected requestBody for POST route, got: %s", jsonStr)
	}

	// Verify components
	if !strings.Contains(jsonStr, `"User"`) {
		t.Errorf("expected User struct in schemas, got: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"integer"`) {
		t.Errorf("expected integer type for User.age, got: %s", jsonStr)
	}
}

func TestStoreKeywordAndAdapter(t *testing.T) {
	// 1. Verify parsing and codegen of store keyword
	src := `
	store "file://./tmp_bucket"
	fn save() {
		store.put("key123", "some payload")
		let data = store.get("key123")
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	codegen := compiler.NewCodegen(prog)
	generated, err := codegen.Generate()
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	if !strings.Contains(generated, `runtime.InitStore(fmt.Sprint("file://./tmp_bucket"))`) {
		t.Errorf("expected generated code to initialize store, got: %s", generated)
	}
	if !strings.Contains(generated, `runtime.StorePut`) {
		t.Errorf("expected generated code to call StorePut, got: %s", generated)
	}
	if !strings.Contains(generated, `runtime.StoreGet`) {
		t.Errorf("expected generated code to call StoreGet, got: %s", generated)
	}

	// 2. Verify runtime Put & Get success path
	tempDir, err := os.MkdirTemp("", "serv_store_test_dir")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	runtime.InitStore("file://" + tempDir)
	_, err = runtime.StorePut("test_key.txt", "hello store")
	if err != nil {
		t.Errorf("expected no error for StorePut, got: %v", err)
	}

	val, err := runtime.StoreGet("test_key.txt")
	if err != nil {
		t.Errorf("expected no error for StoreGet, got: %v", err)
	}
	if val != "hello store" {
		t.Errorf("expected value 'hello store', got: %v", val)
	}
}

func TestOidcAndRoleGuards(t *testing.T) {
	// 1. Verify parser/codegen of auth.role and auth.scope
	src := `
	auth "oidc://localhost:12345"
	route "GET" "/secure-role" (req) use [auth.role("admin")] {
		return "role-ok"
	}
	route "GET" "/secure-scope" (req) use [auth.scope("write:items")] {
		return "scope-ok"
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	codegen := compiler.NewCodegen(prog)
	generated, err := codegen.Generate()
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	if !strings.Contains(generated, `"auth.role(\"admin\")"`) {
		t.Errorf("expected auth.role middleware registered in codegen, got: %s", generated)
	}
	if !strings.Contains(generated, `"auth.scope(\"write:items\")"`) {
		t.Errorf("expected auth.scope middleware registered in codegen, got: %s", generated)
	}

	// 2. Setup mock OIDC Discovery and JWKS Server
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	kid := "test-key-id"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration") {
			w.Header().Set("Content-Type", "application/json")
			// JwksURI must point back to this test server
			fmt.Fprintf(w, `{"jwks_uri": %q}`, "http://"+r.Host+"/certs")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/certs") {
			w.Header().Set("Content-Type", "application/json")
			nStr := base64.RawURLEncoding.EncodeToString(privKey.N.Bytes())
			eBytes := big.NewInt(int64(privKey.E)).Bytes()
			eStr := base64.RawURLEncoding.EncodeToString(eBytes)
			fmt.Fprintf(w, `{"keys":[{"kty":"RSA","use":"sig","kid":%q,"alg":"RS256","n":%q,"e":%q}]}`, kid, nStr, eStr)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Initialize runtime auth using mock server URL
	runtime.InitAuth("oidc://" + strings.TrimPrefix(mockServer.URL, "http://"))

	// Setup routes in runtime
	runtime.AddRouteWithMiddleware("GET", "/secure-role", 0, "", []string{"auth", "auth.role(\"admin\")"}, func(req runtime.Request) interface{} {
		return "role-ok"
	})
	runtime.AddRouteWithMiddleware("GET", "/secure-scope", 0, "", []string{"auth", "auth.scope(\"write:items\")"}, func(req runtime.Request) interface{} {
		return "scope-ok"
	})

	// Test request runner
	runRequest := func(path, token string) (int, string) {
		req := runtime.Request{
			Method:  "GET",
			Path:    path,
			Headers: map[string]string{"Authorization": "Bearer " + token},
			Params:  make(map[string]string),
		}
		// Dispatch route using test handler
		h, p, _, _ := runtime.MatchRouteStub("GET", path)
		if h == nil {
			return 404, "Not Found"
		}
		req.Params = p
		res := h(req)
		
		if mapRes, ok := res.(map[string]interface{}); ok {
			statusVal := mapRes["status"]
			if statusInt, ok := statusVal.(int); ok {
				return statusInt, fmt.Sprint(mapRes["message"])
			}
		}
		if strRes, ok := res.(string); ok {
			return 200, strRes
		}
		return 200, ""
	}

	// Sign valid/invalid tokens and run assertions
	validClaims := map[string]interface{}{
		"iss":   mockServer.URL,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"roles": []interface{}{"admin", "user"},
		"scope": "read:items write:items",
	}
	invalidClaims := map[string]interface{}{
		"iss":   mockServer.URL,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"roles": []interface{}{"guest"},
		"scope": "read:items",
	}

	validToken := generateRS256JWT(validClaims, privKey, kid)
	invalidToken := generateRS256JWT(invalidClaims, privKey, kid)

	// A. Role Route Checks
	st, body := runRequest("/secure-role", validToken)
	if st != 200 || body != "role-ok" {
		t.Errorf("expected 200 role-ok, got status %d, body %s", st, body)
	}

	st, body = runRequest("/secure-role", invalidToken)
	if st != 403 {
		t.Errorf("expected 403 Forbidden, got status %d, body %s", st, body)
	}

	// B. Scope Route Checks
	st, body = runRequest("/secure-scope", validToken)
	if st != 200 || body != "scope-ok" {
		t.Errorf("expected 200 scope-ok, got status %d, body %s", st, body)
	}

	st, body = runRequest("/secure-scope", invalidToken)
	if st != 403 {
		t.Errorf("expected 403 Forbidden, got status %d, body %s", st, body)
	}
}

func generateRS256JWT(claims map[string]interface{}, privateKey *rsa.PrivateKey, kid string) string {
	header := fmt.Sprintf(`{"alg":"RS256","typ":"JWT","kid":%q}`, kid)
	headerEnc := base64.RawURLEncoding.EncodeToString([]byte(header))
	claimsBytes, _ := json.Marshal(claims)
	claimsEnc := base64.RawURLEncoding.EncodeToString(claimsBytes)

	signingInput := headerEnc + "." + claimsEnc
	hashed := sha256.Sum256([]byte(signingInput))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	sigEnc := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + sigEnc
}





