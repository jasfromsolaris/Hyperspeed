package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
		r.Post("/installs/bootstrap-token", handleIssueBootstrapToken(&cfg))
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

type issueBootstrapTokenRequest struct {
	// Optional stable install id; if empty, Worker creates one.
	InstallID string `json:"install_id"`
	// Optional token TTL in seconds (min 60, max 3600 in worker).
	TTLSeconds int `json:"ttl_sec"`
}

type issueBootstrapTokenResponse struct {
	ProvisioningBaseURL       string `json:"provisioning_base_url"`
	ProvisioningInstallID     string `json:"provisioning_install_id"`
	ProvisioningBootstrapToken string `json:"provisioning_bootstrap_token"`
	ExpiresInSec              int    `json:"expires_in_sec"`
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

func handleIssueBootstrapToken(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(cfg.WorkerAdminToken) == "" || strings.TrimSpace(cfg.WorkerAdminURL) == "" {
			writeErr(w, http.StatusServiceUnavailable, "bootstrap_unavailable")
			return
		}
		var body issueBootstrapTokenRequest
		_ = json.NewDecoder(r.Body).Decode(&body)

		reqBody, _ := json.Marshal(map[string]any{
			"install_id": strings.TrimSpace(body.InstallID),
			"ttl_sec":    body.TTLSeconds,
		})
		url := strings.TrimSuffix(cfg.WorkerAdminURL, "/") + "/v1/admin/bootstrap-token"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(reqBody))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "bootstrap_request_failed")
			return
		}
		req.Header.Set("Authorization", "Bearer "+cfg.WorkerAdminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			slog.Error("worker bootstrap issue", "err", err)
			writeErr(w, http.StatusBadGateway, "bootstrap_request_failed")
			return
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			slog.Error("worker bootstrap issue", "status", resp.StatusCode, "body", string(raw))
			writeErr(w, http.StatusBadGateway, "bootstrap_request_failed")
			return
		}

		var out issueBootstrapTokenResponse
		if err := json.Unmarshal(raw, &out); err != nil {
			writeErr(w, http.StatusBadGateway, "bootstrap_request_failed")
			return
		}
		if out.ProvisioningBaseURL == "" {
			out.ProvisioningBaseURL = strings.TrimSpace(cfg.ProvisioningBaseURL)
		}
		if out.ProvisioningBaseURL == "" {
			out.ProvisioningBaseURL = "https://provision-gw.hyperspeedapp.com"
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provisioning_base_url":        out.ProvisioningBaseURL,
			"provisioning_install_id":      out.ProvisioningInstallID,
			"provisioning_bootstrap_token": out.ProvisioningBootstrapToken,
			"expires_in_sec":               out.ExpiresInSec,
			"compose_env": map[string]string{
				"PROVISIONING_BASE_URL":        out.ProvisioningBaseURL,
				"PROVISIONING_BOOTSTRAP_TOKEN": out.ProvisioningBootstrapToken,
			},
		})
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
