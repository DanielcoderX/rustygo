package main

import (
	"fmt"
	"io"
	"net/http"

	rg "rustygo"
	"rustygo/codec"
)

func UserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scope, ok := rg.ScopeFromContext(r.Context())
	if !ok {
		http.Error(w, "Arena scope missing", http.StatusInternalServerError)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Zero-copy JSON scanning directly into arena scope
	scanner := codec.NewJSONScanner(bodyBytes)
	var username string
	for {
		tokType, val, err := scanner.Next(scope)
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "Malformed JSON", http.StatusBadRequest)
			return
		}
		if tokType == codec.JSONTokenKey && val == "user" {
			_, nextVal, err := scanner.Next(scope)
			if err != nil {
				http.Error(w, "Missing user value", http.StatusBadRequest)
				return
			}
			username = nextVal
		}
	}

	w.Header().Set("Content-Type", "application/json")
	// Format response buffer in arena
	resp := fmt.Sprintf(`{"status":"ok","welcome":%q}`, username)
	buf := rg.AllocSlice[byte](scope, len(resp))
	copy(buf, resp)

	_, _ = w.Write(buf)
}

func SetupServer(arena *rg.Arena) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/user", UserHandler)
	return rg.HTTPMiddleware(arena)(mux)
}

func main() {
	arena := rg.NewArena(1024 * 1024)
	defer arena.Close()

	server := SetupServer(arena)
	fmt.Println("Zero-copy HTTP REST server running on :8080")
	_ = http.ListenAndServe(":8080", server)
}
