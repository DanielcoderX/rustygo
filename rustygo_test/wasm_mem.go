package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"rustygo/analyzer"

	"golang.org/x/tools/go/packages"
)

const htmlContent = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Go WASM Memory Benchmark</title>
    <style>
        :root {
            --bg: #0b0f19;
            --card-bg: rgba(255, 255, 255, 0.03);
            --border: rgba(255, 255, 255, 0.08);
            --text: #f3f4f6;
            --text-muted: #9ca3af;
            --primary: #3b82f6;
            --accent: #10b981;
            --accent-glow: rgba(16, 185, 129, 0.15);
            --danger: #ef4444;
            --danger-glow: rgba(239, 68, 68, 0.15);
        }
        body {
            background-color: var(--bg);
            color: var(--text);
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            margin: 0;
            padding: 40px 20px;
            display: flex;
            flex-direction: column;
            align-items: center;
            min-height: 100vh;
        }
        .container {
            max-width: 1000px;
            width: 100%;
        }
        h1 {
            font-size: 2.5rem;
            font-weight: 800;
            text-align: center;
            margin-bottom: 10px;
            background: linear-gradient(to right, #3b82f6, #8b5cf6);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        p.subtitle {
            text-align: center;
            color: var(--text-muted);
            margin-bottom: 40px;
        }
        .grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
            margin-bottom: 40px;
        }
        @media (max-width: 768px) {
            .grid {
                grid-template-columns: 1fr;
            }
        }
        .card {
            background: var(--card-bg);
            border: 1px solid var(--border);
            border-radius: 16px;
            padding: 30px;
            backdrop-filter: blur(10px);
            box-shadow: 0 4px 30px rgba(0, 0, 0, 0.5);
            display: flex;
            flex-direction: column;
            align-items: center;
            transition: transform 0.3s ease, border-color 0.3s ease;
        }
        .card:hover {
            transform: translateY(-5px);
        }
        .card.normal {
            border-color: rgba(239, 68, 68, 0.2);
        }
        .card.normal:hover {
            border-color: rgba(239, 68, 68, 0.5);
        }
        .card.rustygo {
            border-color: rgba(16, 185, 129, 0.2);
        }
        .card.rustygo:hover {
            border-color: rgba(16, 185, 129, 0.5);
        }
        h2 {
            font-size: 1.5rem;
            margin-top: 0;
            margin-bottom: 20px;
        }
        .memory-display {
            font-size: 3.5rem;
            font-weight: 800;
            margin: 20px 0;
            font-variant-numeric: tabular-nums;
        }
        .normal .memory-display {
            color: var(--danger);
            text-shadow: 0 0 20px var(--danger-glow);
        }
        .rustygo .memory-display {
            color: var(--accent);
            text-shadow: 0 0 20px var(--accent-glow);
        }
        .details {
            color: var(--text-muted);
            font-size: 0.9rem;
            text-align: center;
            line-height: 1.6;
        }
        .btn-container {
            display: flex;
            justify-content: center;
            margin-bottom: 40px;
        }
        button {
            background: linear-gradient(135deg, var(--primary), #8b5cf6);
            color: white;
            border: none;
            padding: 16px 40px;
            font-size: 1.1rem;
            font-weight: 600;
            border-radius: 30px;
            cursor: pointer;
            box-shadow: 0 4px 20px rgba(59, 130, 246, 0.4);
            transition: all 0.2s ease;
        }
        button:hover {
            transform: scale(1.03);
            box-shadow: 0 6px 24px rgba(59, 130, 246, 0.6);
        }
        button:active {
            transform: scale(0.98);
        }
        button:disabled {
            background: #4b5563;
            cursor: not-allowed;
            box-shadow: none;
            transform: none;
        }
        .status {
            text-align: center;
            font-weight: 500;
            color: var(--text-muted);
            margin-top: 20px;
        }
    </style>
    <script src="/wasm_exec.js"></script>
</head>
<body>
    <div class="container">
        <h1>Go WASM Memory Comparison</h1>
        <p class="subtitle">Allocating 55,000,000 structures per batch to push peak linear memory to ~3.8 GB</p>

        <div class="btn-container">
            <button id="runBtn" onclick="runBenchmark()">Run Benchmark</button>
        </div>

        <div class="status" id="statusMsg">Ready to run benchmark</div>

        <div class="grid">
            <div class="card normal">
                <h2>Normal Go WASM</h2>
                <div class="memory-display" id="normalMem">-</div>
                <div class="details">
                    Uses Go's default heap allocation.<br>
                    Garbage collector retains WASM memory.
                </div>
            </div>

            <div class="card rustygo">
                <h2>Rustygo WASM</h2>
                <div class="memory-display" id="rustygoMem">-</div>
                <div class="details">
                    AST rewritten to use bulk Arena allocation.<br>
                    Memory is recycled instantly.
                </div>
            </div>
        </div>
    </div>

    <script>
        async function runWasm(url) {
            const response = await fetch(url);
            const buffer = await response.arrayBuffer();
            const go = new Go();
            const { instance } = await WebAssembly.instantiate(buffer, go.importObject);
            
            // Run WASM workload
            await go.run(instance);

            // Get final linear memory size in MB
            const bytes = instance.exports.mem.buffer.byteLength;
            return (bytes / (1024 * 1024)).toFixed(2) + " MB";
        }

        async function runBenchmark() {
            const btn = document.getElementById("runBtn");
            const status = document.getElementById("statusMsg");
            btn.disabled = true;
            
            try {
                status.innerText = "Running Normal Go WASM...";
                document.getElementById("normalMem").innerText = "Running...";
                // Small delay to allow UI to update
                await new Promise(r => setTimeout(r, 50)); 

                const normalMem = await runWasm("/normal.wasm");
                document.getElementById("normalMem").innerText = normalMem;

                status.innerText = "Running Rustygo WASM...";
                document.getElementById("rustygoMem").innerText = "Running...";
                await new Promise(r => setTimeout(r, 50));

                const rustygoMem = await runWasm("/rewritten.wasm");
                document.getElementById("rustygoMem").innerText = rustygoMem;

                status.innerText = "Benchmark Completed Successfully!";
            } catch (err) {
                console.error(err);
                status.innerText = "Benchmark failed: " + err.message;
            } finally {
                btn.disabled = false;
            }
        }
    </script>
</body>
</html>
`

func main() {
	tempDir, err := os.MkdirTemp("", "wasm-mem-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Write the workload program
	srcPath := filepath.Join(tempDir, "main.go")
	src := `package main

import (
	"fmt"
	_ "rustygo"
)

type Element struct {
	ID   int
	Data [8]int
}

func runBatch() int {
    // Increased to 55 Million to push memory usage near the 4GB WASM limit 
	slice := make([]Element, 55000000)
	for i := 0; i < 55000000; i++ {
		slice[i].ID = i
		slice[i].Data[0] = i * 2
	}
	return slice[10000000].Data[0]
}

func main() {
	var sum int
	for batch := 0; batch < 5; batch++ {
		sum += runBatch()
	}
	fmt.Println("Workload sum:", sum)
}
`
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		panic(err)
	}

	// 2. Load and rewrite the AST using analyzer
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Fset: fset,
		Mode: packages.LoadAllSyntax,
		Dir:  tempDir,
	}
	pkgs, err := packages.Load(cfg, srcPath)
	if err != nil || len(pkgs) == 0 {
		panic(fmt.Errorf("failed to load package: %v", err))
	}
	pkg := pkgs[0]

	funcDecls := make(map[string]*ast.FuncDecl)
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			if fd, ok := n.(*ast.FuncDecl); ok {
				if obj, ok := pkg.TypesInfo.ObjectOf(fd.Name).(*types.Func); ok {
					funcDecls[obj.FullName()] = fd
				}
				return false
			}
			return true
		})
	}

	// Scaled Rustygo's internal Arena space to 2.5GB to accommodate the massive 55M structs safely.
	outBytes, changed, err := analyzer.RewriteFileWithConfig(pkg.Fset, pkg.Syntax[0], pkg.TypesInfo, pkg.PkgPath, analyzer.RewriteConfig{ArenaBytes: 2500 * 1024 * 1024}, funcDecls)
	if err != nil {
		panic(err)
	}
	if !changed {
		panic("expected source to be rewritten")
	}

	rewrittenPath := filepath.Join(tempDir, "rewritten.go")
	if err := os.WriteFile(rewrittenPath, outBytes, 0644); err != nil {
		panic(err)
	}

	// 3. Compile both WASM files
	normalWasm := filepath.Join(tempDir, "normal.wasm")
	fmt.Println("Compiling normal WASM...")
	cmd := exec.Command("go", "build", "-o", normalWasm, srcPath)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	rewrittenWasm := filepath.Join(tempDir, "rewritten.wasm")
	fmt.Println("Compiling rewritten WASM...")
	cmd = exec.Command("go", "build", "-o", rewrittenWasm, rewrittenPath)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	// 4. Set local paths (Checking both the new Go 1.24+ 'lib' path and old 'misc' path)
	goroot := runtime.GOROOT()
	wasmExecJS := filepath.Join(goroot, "lib", "wasm", "wasm_exec.js") // New default
	if _, err := os.Stat(wasmExecJS); os.IsNotExist(err) {
		wasmExecJS = filepath.Join(goroot, "misc", "wasm", "wasm_exec.js") // Legacy fallback
	}

	// 5. Start HTTP Server
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(htmlContent))
	})

	http.HandleFunc("/wasm_exec.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")

		// If the local file actually exists, use it
		if _, err := os.Stat(wasmExecJS); err == nil {
			http.ServeFile(w, r, wasmExecJS)
			return
		}

		// Fallback: Stream it dynamically from Go's updated location on GitHub
		upstreamURL := "https://fastly.jsdelivr.net/gh/golang/go@master/lib/wasm/wasm_exec.js"
		fmt.Println("Local wasm_exec.js missing. Fetching from new upstream path on GitHub...")

		resp, err := http.Get(upstreamURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			http.Error(w, "Failed to fetch wasm_exec.js from upstream", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		_, _ = io.Copy(w, resp.Body)
	})

	http.HandleFunc("/normal.wasm", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, normalWasm)
	})
	http.HandleFunc("/rewritten.wasm", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, rewrittenWasm)
	})

	fmt.Println("\n==================================================")
	fmt.Println("WASM Memory Benchmark Server started at http://localhost:8888")
	fmt.Println("Open this link in your browser to run the benchmark")
	fmt.Println("==================================================")

	if err := http.ListenAndServe(":8888", nil); err != nil {
		panic(err)
	}
}