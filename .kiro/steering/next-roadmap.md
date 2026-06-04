# Serv Next Roadmap

Tracking remaining work to make Serv production-ready and competitive.

## Status Legend
- ⬜ Not started
- 🟡 In progress
- ✅ Done

---

## Developer Experience

| Status | Item | Description | Priority |
|--------|------|-------------|----------|
| ✅ | Package manager | `serv add <pkg>` — auto-generates `.srv.d` declarations from Go packages | High |
| ✅ | REPL | `serv repl` — interactive shell for quick experiments | Medium |
| ✅ | Formatter | `serv fmt` — opinionated auto-formatter | Medium |
| ⬜ | Playground | Web-based editor (like Go Playground) | Low |
| ✅ | Better errors | Diagnostics with suggestions ("did you mean X?") | Medium |

---

## Language Completeness

| Status | Item | Description | Priority |
|--------|------|-------------|----------|
| ✅ | String methods | `.split()`, `.trim()`, `.replace()`, `.startsWith()`, `.includes()`, `.toUpper()`, `.toLower()` | High |
| ✅ | Closures / arrow fns | `let double = fn(x) { return x * 2 }` and `x => x * 2` shorthand | High |
| ✅ | Destructuring | `let { name, email } = user` | Medium |
| ✅ | Optional chaining | `user?.address?.city` — returns nil if any part is nil | Medium |
| ✅ | Spread operator | `let merged = { ...defaults, ...overrides }` | Medium |
| ✅ | Enums with values | `enum Status { Active = 1, Inactive = 2 }` | Low |
| ✅ | Type aliases | `type UserID = int` | Low |
| ✅ | Generic constraints | `fn sort[T: Comparable](items: []T)` | Low |

---

## Production Readiness

| Status | Item | Description | Priority |
|--------|------|-------------|----------|
| ✅ | Structured logging | JSON log output, log levels, context fields | High |
| ✅ | OpenTelemetry | Built-in tracing/metrics export (OTLP) | Medium |
| ✅ | Health endpoints | Auto-generated `/health` and `/ready` | High |
| ✅ | Config validation | Schema validation at startup, fail fast | Medium |
| ✅ | TLS support | `server "8080" tls "cert.pem" "key.pem"` | Medium |
| ✅ | WebSocket support | `ws "/chat" (conn) { ... }` | High |
| ⬜ | Graceful hot-reload | Zero-downtime restarts in watch mode | Low |
| ✅ | Request validation | Built-in body/param validation with schema | Medium |

---

## Ecosystem & Distribution

| Status | Item | Description | Priority |
|--------|------|-------------|----------|
| ✅ | Documentation site | Auto-generated docs from `.srv` source | Medium |
| ✅ | CI/CD templates | GitHub Actions, GitLab CI configs | Low |
| ⬜ | Docker base image | `FROM serv:latest` for easy containerization | Low |
| ⬜ | Homebrew/Scoop | `brew install serv` / `scoop install serv` | Medium |
| ✅ | Standard library | Importable `.srv` modules (auth, validation, pagination) | Medium |
| ⬜ | VS Code Marketplace | Publish extension publicly | Medium |

---

## Suggested Implementation Order

### Sprint 1: Language Ergonomics
1. String methods (`.split`, `.trim`, `.replace`, etc.)
2. Closures / arrow functions (`x => x * 2`)
3. Health endpoints (auto `/health` and `/ready`)

### Sprint 2: Ecosystem Access
4. Package manager (`serv add`)
5. WebSocket support (`ws` keyword)
6. Structured logging (JSON mode)

### Sprint 3: Production Polish
7. TLS support
8. Optional chaining (`?.`)
9. Destructuring
10. Formatter (`serv fmt`)

### Sprint 4: Distribution
11. Homebrew/Scoop packages
12. VS Code Marketplace publish
13. Documentation site
14. Standard library modules
