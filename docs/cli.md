# CLI Reference

## serv build

Compile a `.srv` file to a native binary.

```bash
serv build <file.srv> [-o <output>]
```

**Examples:**
```bash
serv build app.srv                    # → service.exe
serv build app.srv -o myapp.exe       # Custom output name
```

## serv run

Compile and run immediately.

```bash
serv run <file.srv> [--watch]
```

**Options:**
- `--watch` — Watch for file changes and hot-reload

## serv test

Run tests defined in a `.srv` file.

```bash
serv test <file.srv>
```

Runs all `test "name" { ... }` blocks and reports results.

## serv lint

Check syntax without building.

```bash
serv lint <file.srv>
```

## serv fmt

Format a `.srv` file (4-space indent, consistent style).

```bash
serv fmt <file.srv>            # Format in place
serv fmt --check <file.srv>    # Check only (exit 1 if unformatted)
```

## serv repl

Interactive Serv shell.

```bash
serv repl
```

**Commands inside REPL:**
- Type any expression to evaluate: `1 + 2`, `"hello".toUpper()`
- `let x = 42` — declare variables (persisted across lines)
- `state` — show all declarations
- `clear` — reset state
- `exit` — quit

## serv add

Generate a `.srv.d` declaration file for a Go package.

```bash
serv add <go-package-path>
```

**Examples:**
```bash
serv add github.com/google/uuid
serv add encoding/json
serv add net/url
```

Downloads the package (if needed) and generates type declarations in `declarations/`.

## serv packages

List installed package declarations.

```bash
serv packages
```

## serv remove

Remove a package declaration.

```bash
serv remove <package-name>
```

## serv dockerize

Generate a Dockerfile for deployment.

```bash
serv dockerize <file.srv>
```

## Runtime Flags

Compiled Serv binaries accept these flags:

```bash
./myservice.exe --port 9090     # Override server port
./myservice.exe --mcp           # Start as MCP tool server
```

**Environment variables:**
- `PORT` — Override server port
- `LOG_FORMAT=json` — JSON log output
- `LOG_LEVEL=debug` — Set log level
- `OTEL_ENDPOINT=http://localhost:4318` — Enable OpenTelemetry
- `OTEL_SERVICE_NAME=my-service` — Service name for traces
