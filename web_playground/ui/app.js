// App logic for Serv Web Playground

let editor = null;
let isMonacoReady = false;
let isWasmLoaded = false;

const DEFAULT_CODE = `// Serv Web Playground
// Write, format, and run Serv background services here.

log.info("Hello from Serv Web Playground!")
let msg = greet("Developer")
log.info(msg)

fn greet(name: string) -> string {
    return "Greetings, " + name + "!"
}
`;

// Initialize Monaco Editor
require.config({ paths: { vs: 'https://cdnjs.cloudflare.com/ajax/libs/monaco-editor/0.39.0/min/vs' } });
require(['vs/editor/editor.main'], function () {
    // Register Serv language if needed, or configure basic JS-like colorization
    monaco.languages.register({ id: 'serv' });
    monaco.languages.setMonarchTokensProvider('serv', {
        tokenizer: {
            root: [
                [/\b(fn|let|return|import|export|from|try|catch|match|test|assert|enum|struct|interface|middleware|if|else|for|in|spawn|every|cron|subscribe|publish|true|false|nil|self|await|ws|validate|type|use|limit|tool)\b/, 'keyword'],
                [/\b(log|fmt|print|println)\b/, 'type'],
                [/\b[0-9]+\b/, 'number'],
                [/"([^"\\]|\\.)*"/, 'string'],
                [/\/\/.*$/, 'comment'],
            ]
        }
    });

    editor = monaco.editor.create(document.getElementById('monaco-container'), {
        value: DEFAULT_CODE,
        language: 'serv',
        theme: 'vs-dark',
        automaticLayout: true,
        fontSize: 14,
        fontFamily: "'JetBrains Mono', Consolas, monospace",
        minimap: { enabled: false },
        lineNumbersMinChars: 3,
        scrollbar: {
            verticalScrollbarSize: 8,
            horizontalScrollbarSize: 8
        }
    });

    isMonacoReady = true;
    checkSystemStatus();
});

// Fallback initialization if Monaco doesn't load
setTimeout(() => {
    if (!isMonacoReady) {
        console.warn("Monaco failed to load within timeout, falling back to textarea");
        document.getElementById('monaco-container').style.display = 'none';
        const fallback = document.getElementById('fallback-editor');
        fallback.style.display = 'block';
        fallback.value = DEFAULT_CODE;
        checkSystemStatus();
    }
}, 5000);

// Helper to get current code
function getCodeValue() {
    if (isMonacoReady && editor) {
        return editor.getValue();
    }
    return document.getElementById('fallback-editor').value;
}

// Helper to set code value
function setCodeValue(val) {
    if (isMonacoReady && editor) {
        editor.setValue(val);
    } else {
        document.getElementById('fallback-editor').value = val;
    }
}

// Check system initialization status
function checkSystemStatus() {
    const status = document.getElementById('wasm-status');
    if (isWasmLoaded) {
        status.textContent = "WASM Ready";
        status.className = "status-indicator ready";
    } else {
        status.textContent = "Offline Formatter (WASM Loading...)";
        status.className = "status-indicator";
    }
}

// Load WebAssembly
const go = new Go();
WebAssembly.instantiateStreaming(fetch("serv.wasm"), go.importObject)
    .then((result) => {
        go.run(result.instance);
        isWasmLoaded = true;
        checkSystemStatus();
        printToTerminal("Serv Compiler WebAssembly loaded successfully.", "success-msg");
    })
    .catch((err) => {
        console.error("WASM failed to load: ", err);
        const status = document.getElementById('wasm-status');
        status.textContent = "WASM Load Failed";
        status.className = "status-indicator error";
        printToTerminal("WASM compilation engine could not load. Syntax analysis and local formatting are disabled, but 'Run' is still available.", "warning-msg");
    });

// Terminal printer helpers
function printToTerminal(text, className = "stdout-msg") {
    const term = document.getElementById('terminal');
    const div = document.createElement('div');
    div.className = `terminal-line ${className}`;
    div.textContent = text;
    term.appendChild(div);
    term.scrollTop = term.scrollHeight;
}

function clearTerminal() {
    document.getElementById('terminal').innerHTML = '';
}

// Wire up events
document.getElementById('btn-clear').addEventListener('click', clearTerminal);

document.getElementById('btn-format').addEventListener('click', () => {
    if (!isWasmLoaded) {
        printToTerminal("WASM formatting engine is not loaded yet.", "error-msg");
        return;
    }

    const currentCode = getCodeValue();
    try {
        const res = formatServ(currentCode);
        if (res.error) {
            printToTerminal(`Formatting failed: ${res.error}`, "error-msg");
        } else if (res.formatted) {
            setCodeValue(res.formatted);
            printToTerminal("Formatted successfully.", "success-msg");
        }
    } catch (e) {
        printToTerminal(`Error executing format: ${e.message}`, "error-msg");
    }
});

document.getElementById('btn-run').addEventListener('click', async () => {
    const code = getCodeValue();
    printToTerminal("Compiling & executing code on sandbox server...", "system-msg");

    // Before sending to server, let's run a quick local validation in WASM if available
    if (isWasmLoaded) {
        try {
            const validation = compileServ(code);
            if (validation.error) {
                printToTerminal("Local Compilation Check: Failed", "error-msg");
                printToTerminal(validation.output, "error-msg");
                return; // Stop execution on local compilation check failure
            } else if (validation.analysisOutput) {
                printToTerminal("Analysis Warnings/Diagnostics:", "warning-msg");
                printToTerminal(validation.analysisOutput, "warning-msg");
            }
        } catch (e) {
            console.error("Local compile check error: ", e);
        }
    }

    try {
        const response = await fetch('/api/run', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ source: code })
        });

        const data = await response.json();
        if (response.status !== 200) {
            printToTerminal(`Server returned status ${response.status}: ${data.error || 'Unknown error'}`, "error-msg");
            if (data.output) {
                printToTerminal(data.output, "stderr-msg");
            }
            return;
        }

        if (data.output) {
            printToTerminal(data.output, "stdout-msg");
        }
        if (data.success) {
            printToTerminal("Program exited successfully.", "success-msg");
        } else {
            printToTerminal(`Program execution failed: ${data.error || 'Execution returned non-zero exit code'}`, "error-msg");
        }
    } catch (err) {
        printToTerminal(`Failed to connect to execution server: ${err.message}`, "error-msg");
    }
});
