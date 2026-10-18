package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	rg "rustygo"
	"rustygo/codec"
)

// GatewayServer coordinates zero-copy API gateway routing.
type GatewayServer struct {
	cache *rg.ArenaCache
	pool  *rg.ShardedArenaPool
}

func NewGatewayServer() *GatewayServer {
	return &GatewayServer{
		cache: rg.NewArenaCache(16 * 1024 * 1024), // 16MB per generation
		pool:  rg.NewShardedArenaPool(64*1024, 32),
	}
}

// ProxyHandler inspects incoming requests, queries the generational cache, and returns zero-copy responses.
func (g *GatewayServer) ProxyHandler(w http.ResponseWriter, r *http.Request) {
	scope, ok := rg.ScopeFromContext(r.Context())
	if !ok {
		http.Error(w, "scope missing", http.StatusInternalServerError)
		return
	}

	key := r.URL.Path
	// Check generational LRU cache without GC pointer scanning
	if cachedVal, found := g.cache.GetScope(key, scope); found {
		w.Header().Set("X-Cache", "HIT-ZERO-GC")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cachedVal)
		return
	}

	// Read and parse incoming request body using SWAR vector JSON scanner
	if r.Method == http.MethodPost {
		body, err := rg.ReadResponseBody(&http.Response{Body: r.Body}, scope, 1024*1024)
		if err == nil && len(body) > 0 {
			scanner := codec.NewJSONScanner(body)
			for {
				tok, val, err := scanner.Next(scope)
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
				_ = tok
				_ = val
			}
		}
	}

	// Generate response payload inside arena scope
	buf := rg.NewArenaBuffer(scope, 256)
	buf.WriteString(`{"status":"success","gateway":"RustyGo v1.0.0","path":`)
	buf.WriteString(fmt.Sprintf("%q", key))
	buf.WriteString(`}`)

	respData := buf.Bytes()
	g.cache.Set(key, respData)

	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(respData)
}

func main() {
	gw := NewGatewayServer()
	mux := http.NewServeMux()

	// 1. Core API endpoint wrapped in RustyGo request-scoped arena middleware
	arena := rg.NewArena(128 * 1024)
	defer arena.Close()

	mux.Handle("/api/", rg.HTTPMiddleware(arena)(http.HandlerFunc(gw.ProxyHandler)))

	// 2. Telemetry and live memory dashboard
	mux.Handle("/metrics", rg.MetricsHandler())
	rg.RegisterDebugHandler(mux)

	log.Println("🚀 RustyGo v1.0.0 Gateway running on :8080")
	log.Println("📊 Prometheus metrics : http://localhost:8080/metrics")
	log.Println("⚡ Live Arena Inspector: http://localhost:8080/debug/rustygo/arenas")

	log.Fatal(http.ListenAndServe(":8080", mux))
}
