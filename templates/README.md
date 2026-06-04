# Serv Project Templates

## Kiro + Serv (AI-Assisted Development)

To create a new Serv project with Kiro support:

1. Copy the `kiro-project/` template to your new project directory:
   ```bash
   cp -r templates/kiro-project/ ~/my-new-service/
   cd ~/my-new-service/
   ```

2. Open in Kiro — the steering files tell Kiro how to write Serv code:
   - `.kiro/steering/serv.md` — Language syntax & API reference
   - `.kiro/steering/project.md` — Project conventions & patterns

3. Start building:
   ```bash
   serv run main.srv --watch
   ```

## What the Steering Files Provide

When Kiro sees these files, it automatically:
- Writes valid `.srv` syntax (not Go, not TypeScript)
- Uses the correct built-in objects (`log`, `db`, `cache`, `http`, etc.)
- Follows the `?` operator pattern for error handling
- Imports from `stdlib/` correctly
- Runs `serv build` / `serv test` / `serv lint` for verification
- Uses 4-space indentation and `serv fmt` conventions

## Without Kiro

The template works as a regular Serv project too — just ignore the `.kiro/` directory.
