# Serv Language Roadmap: TypeScript-for-Go

Serv aims to be to Go what TypeScript is to JavaScript — same runtime, better developer experience, stronger compile-time guarantees.

## Design Principles

- **Gradual typing**: Untyped code still works. Types are additive, not mandatory.
- **Structural typing**: If it has the methods, it satisfies the interface. No explicit `implements`.
- **Null safety**: `nil` is only assignable to optional types (`T?` or `T | nil`). Strict by default.
- **Readable output**: Generated Go should be clean enough to debug directly.
- **Full Go interop**: Any Go package should be callable via declarations.

## Phase 1: Structs & Methods ✅ (current)

```srv
struct User {
    id: int
    name: string
    email: string
    active: bool
}

fn User.fullName() -> string {
    return f"{self.name} ({self.email})"
}

let u = User { id: 1, name: "Alice", email: "alice@example.com", active: true }
log.info(u.fullName())
```

## Phase 2: Error Handling & Multi-Return

```srv
fn fetchUser(id: int) -> User | error {
    let row, err = db.query("SELECT * FROM users WHERE id = ?", id)
    if err != nil {
        return err
    }
    return User { id: row.id, name: row.name, email: row.email, active: true }
}
```

## Phase 3: Module System

```srv
export struct User { ... }
import { User } from "./models/user.srv"
```

## Phase 4: Interfaces

```srv
interface Serializable {
    fn serialize() -> string
}

// Structural: User satisfies Serializable if it has serialize()
fn User.serialize() -> string {
    return json.stringify(self)
}
```

## Phase 5: Generics

```srv
fn filter<T>(items: []T, pred: fn(T) -> bool) -> []T { ... }
fn map<T, U>(items: []T, transform: fn(T) -> U) -> []U { ... }
```

## Phase 6: Collection Methods & Arrow Functions

```srv
let active = users.filter(u => u.active).map(u => u.name)
```

## Phase 7: Go Package Declarations

```srv
// uuid.srv.d
declare module "github.com/google/uuid" {
    fn New() -> string
}
```

## Phase 8: Async/Await

```srv
async fn loadDashboard(id: int) -> Dashboard {
    let [user, orders] = await all([fetchUser(id), fetchOrders(id)])
    return Dashboard { user, orders }
}
```

## Phase 9: Middleware & Decorators

```srv
middleware auth(req) { ... }
route "GET" "/admin" (req) use [auth] { ... }
```

## Phase 10: LSP Server

Full IDE support: autocomplete, go-to-definition, inline errors, refactoring.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Typing | Structural | Matches Go, simpler codegen |
| Null safety | Strict (`T?` for optional) | Major DX win over Go |
| Methods | `fn Type.method()` | Familiar to Go devs |
| Visibility | `export` keyword | Familiar to TS/JS devs |
| Arrow fns | `x => expr` | Essential for collection chains |
| Error model | Union returns `T \| error` | Explicit, no hidden panics |
