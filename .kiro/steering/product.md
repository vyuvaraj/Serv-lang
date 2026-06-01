# Product: Serv

Serv is a domain-specific programming language (DSL) for building background services, schedulers, event-driven applications, and API microservices. Source files use the `.srv` extension.

## What It Does

- Compiles `.srv` source files into native Go code, then into standalone binaries
- Provides declarative syntax for common backend patterns: HTTP routes, cron/interval schedulers, pub/sub messaging, database queries, caching, and concurrency
- Supports optional static typing (`int`, `string`, `bool`) that maps directly to Go primitives
- Includes Python interoperability via `extern fn` bindings
- Has a built-in test framework (`test` blocks with `assert`)
- Offers a CLI with `build`, `run`, `run --watch`, `test`, `lint`, and `dockerize` commands

## Target Users

Developers building lightweight backend services who want a concise, high-level syntax without managing raw Go boilerplate.
