# Project Structure

```
Serv-lang/
├── main.go                  # CLI entry point (build/run/test/lint/dockerize commands)
├── go.mod / go.sum          # Go module definition and dependency lock
├── compiler/                # Compiler pipeline
│   ├── lexer.go             # Tokenizer — converts .srv source to tokens
│   ├── ast.go               # AST node definitions (statements + expressions)
│   ├── parser.go            # Pratt parser — tokens to AST
│   └── codegen.go           # Code generator — AST to Go source
├── runtime/
│   └── runtime.go           # Runtime library linked into generated binaries
│                            #   (HTTP server, broker, DB, cache, scheduler, Python bridge)
├── examples/                # Example .srv programs demonstrating language features
├── scripts/
│   └── analyzer.py          # Python script used by extern fn examples
├── release-scripts/
│   └── build_release.ps1    # Cross-platform release packager
├── vscode-support/
│   └── extension/           # VS Code extension (syntax highlighting, snippets, language config)
├── test_sample.srv          # Sample test file
└── main.srv                 # Sample service entry point
```

## Key Conventions

- All compiler logic lives in the `compiler/` package — lexer, parser, AST, and codegen are separate files by concern.
- The `runtime/` package is a single-file library that gets imported by generated Go code. It provides all built-in capabilities (HTTP, DB, cache, broker, scheduler, Python interop).
- Example files in `examples/` are numbered (`01_`, `02_`, ...) and each demonstrates a specific language feature.
- Generated build artifacts go into `.build/` (gitignored).
- The VS Code extension under `vscode-support/extension/` provides `.srv` syntax highlighting via TextMate grammar.
