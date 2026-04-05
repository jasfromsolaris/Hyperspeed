package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"

	"hyperspeed/control-plane/internal/audit"
	"hyperspeed/control-plane/internal/cf"
	"hyperspeed/control-plane/internal/config"
	"hyperspeed/control-plane/internal/validate"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	dir := filepath.Dir(cfg.AuditDBPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Error("mkdir audit", "err", err)
			os.Exit(1)
		}
	}
	ctx := context.Background()
	auditStore, err := audit.Open(ctx, cfg.AuditDBPath)
	if err != nil {
		slog.Error("audit db", "err", err)
		os.Exit(1)
	}
	defer auditStore.Close()

	cfClient := &cf.Client{
		Token:  cfg.CloudflareAPIToken,
		ZoneID: cfg.CloudflareZoneID,
	}

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Logger)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/v1", func(r chi.Router) {
		r.Use(httprate.Limit(60, 1*time.Minute))
		r.Use(bearerAuth(cfg.BearerToken))
		r.Post("/claims", handleClaim(&cfg, cfClient, auditStore))
		r.Delete("/claims/{slug}", handleDelete(&cfg, cfClient, auditStore))
	})

	addr := cfg.HTTPAddr
	slog.Info("control-plane listening", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

func bearerAuth(want string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			got = strings.TrimSpace(got)
			if got == "" || got != want {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type claimBody struct {
	Slug string `json:"slug"`
	IPv4 string `json:"ipv4"`
}

func handleClaim(cfg *config.Config, client *cf.Client, store *audit.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body claimBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_json")
			return
		}
		slug := strings.ToLower(strings.TrimSpace(body.Slug))
		ipv4 := strings.TrimSpace(body.IPv4)
		if !validate.Slug(slug) {
			writeErr(w, http.StatusBadRequest, "invalid_slug")
			return
		}
		if !validate.IPv4(ipv4) {
			writeErr(w, http.StatusBadRequest, "invalid_ipv4")
			return
		}
		recordName := "www." + slug
		fqdn := giftedTeamRecordFQDN(slug, cfg.BaseDomain)
		ctx := r.Context()
		recordID, err := client.UpsertA(ctx, recordName, fqdn, ipv4, cfg.Proxied)
		if err != nil {
			slog.Error("upsert dns", "err", err)
			writeErr(w, http.StatusBadGateway, "cloudflare_error")
			return
		}
		if err := store.UpsertClaim(ctx, slug, ipv4, recordID); err != nil {
			slog.Error("audit", "err", err)
			writeErr(w, http.StatusInternalServerError, "audit_failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"slug":      slug,
			"fqdn":      fqdn,
			"ipv4":      ipv4,
			"record_id": recordID,
			"https_url": "https://" + fqdn,
		})
	}
}

func handleDelete(cfg *config.Config, client *cf.Client, store *audit.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "slug")))
		if !validate.Slug(slug) {
			writeErr(w, http.StatusBadRequest, "invalid_slug")
			return
		}
		ctx := r.Context()
		recordID, ok, err := store.GetRecordID(ctx, slug)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "audit_failed")
			return
		}
		if !ok {
			fqdn := giftedTeamRecordFQDN(slug, cfg.BaseDomain)
			legacy := slug + "." + cfg.BaseDomain
			existing, err := client.ListARecordsAny(ctx, "www."+slug, fqdn, legacy)
			if err != nil || len(existing) == 0 {
				writeErr(w, http.StatusNotFound, "not_found")
				return
			}
			recordID = existing[0].ID
		}
		if err := client.DeleteRecord(ctx, recordID); err != nil {
			slog.Error("delete dns", "err", err)
			writeErr(w, http.StatusBadGateway, "cloudflare_error")
			return
		}
		_ = store.DeleteClaim(ctx, slug)
		w.WriteHeader(http.StatusNoContent)
	}
}

// giftedTeamRecordFQDN is the public hostname for team sites (e.g. www.acme.hyperspeedapp.com).
func giftedTeamRecordFQDN(slug, baseDomain string) string {
	return "www." + slug + "." + baseDomain
}

func writeErr(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]string{"error": code})
	_, _ = w.Write(b)
}
