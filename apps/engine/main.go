package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Overload()
	port := envOr("PORT", "4100")

	conn, err := openClickHouse()
	if err != nil {
		log.Fatalf("clickhouse: %v", err)
	}
	defer conn.Close()
	log.Printf("connected to ClickHouse database=%s", envOr("CLICKHOUSE_DATABASE", "insightiq"))

	cache := &invCache{byID: map[string]*Investigation{}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		rows, err := conn.QueryMaps(ctx, `SELECT count() AS n FROM alerts_live`)
		n := 0.0
		if err == nil && len(rows) > 0 {
			n = asFloat(rows[0]["n"])
		}
		writeJSON(w, map[string]any{
			"ok":      err == nil,
			"service": "insightiq-engine",
			"database": envOr("CLICKHOUSE_DATABASE", "insightiq"),
			"alerts":  n,
			"error":   errString(err),
		})
	})

	mux.HandleFunc("POST /investigate", func(w http.ResponseWriter, r *http.Request) {
		var req InvestigateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		inv, err := runInvestigation(ctx, conn, req)
		if err != nil {
			log.Printf("investigate error: %v", err)
			http.Error(w, `{"error":"investigate_failed","detail":"`+escape(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		cache.put(inv)
		writeJSON(w, inv)
	})

	mux.HandleFunc("GET /investigations/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if inv := cache.get(id); inv != nil {
			writeJSON(w, inv)
			return
		}
		// Cache miss: rebuild from id pattern inv-{metric}-{YYYYMMDD}
		req, err := requestFromInvestigationID(id)
		if err != nil {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		inv, err := runInvestigation(ctx, conn, req)
		if err != nil {
			log.Printf("investigate rebuild error: %v", err)
			http.Error(w, `{"error":"investigate_failed","detail":"`+escape(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		cache.put(inv)
		writeJSON(w, inv)
	})

	mux.HandleFunc("GET /alerts", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()
		alerts, err := detectAlerts(ctx, conn, cache)
		if err != nil {
			log.Printf("alerts error: %v", err)
			http.Error(w, `{"error":"alerts_failed","detail":"`+escape(err.Error())+`"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, alerts)
	})

	addr := ":" + port
	log.Printf("InsightIQ engine listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

type invCache struct {
	mu   sync.RWMutex
	byID map[string]*Investigation
}

func (c *invCache) put(inv *Investigation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byID[inv.ID] = inv
}

func (c *invCache) get(id string) *Investigation {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byID[id]
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func escape(s string) string {
	b, _ := json.Marshal(s)
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return s
}
