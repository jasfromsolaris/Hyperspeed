package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/redis/go-redis/v9"

	"hyperspeed/api/internal/agenttools"
	"hyperspeed/api/internal/auth"
	"hyperspeed/api/internal/chatai"
	"hyperspeed/api/internal/config"
	"hyperspeed/api/internal/cursor/agents"
	"hyperspeed/api/internal/db"
	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/files"
	"hyperspeed/api/internal/httpx"
	hsmw "hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/migrate"
	"hyperspeed/api/internal/openrouter"
	"hyperspeed/api/internal/overduetasks"
	"hyperspeed/api/internal/rest"
	"hyperspeed/api/internal/store"
	"hyperspeed/api/internal/terminal"
	"hyperspeed/api/internal/version"
	"hyperspeed/api/internal/ws"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	logLevel := slog.LevelInfo
	logHandler := slog.Handler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	if cfg.Debug {
		logLevel = slog.LevelDebug
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	log := slog.New(logHandler)
	slog.SetDefault(log)

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := migrate.Up(ctx, pool); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	st := store.New(pool)
	if n, err := st.CountOrganizations(ctx); err != nil {
		log.Error("count organizations", "err", err)
		os.Exit(1)
	} else if n > 1 {
		log.Warn("multiple organizations in database; this deployment supports at most one organization per database — new organizations cannot be created until only one remains (manual cleanup may be required)", "organization_count", n)
	}
	if err := st.BackfillServiceAccountMemberRoles(ctx); err != nil {
		log.Error("backfill service account roles", "err", err)
	}
	if err := st.BackfillDefaultServiceAccountProfiles(ctx); err != nil {
		log.Error("backfill default AI staff profiles", "err", err)
	}
	if err := st.BackfillEmptyLatestServiceAccountProfiles(ctx); err != nil {
		log.Error("backfill empty AI staff profile versions", "err", err)
	}
	authSvc := auth.NewService(st, cfg.JWTSecret)

	rdbOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Error("redis url", "err", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(rdbOpts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Error("redis ping", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	bus := &events.Bus{Rdb: rdb}

	osCfg, _ := files.FromEnv()
	objStore, err := files.New(ctx, osCfg)
	if err != nil {
		log.Error("object store", "err", err)
		os.Exit(1)
	}
	_ = objStore.EnsureBucket(ctx)

	provH := &rest.ProvisionHandler{
		Store:         st,
		BaseURL:       cfg.ProvisioningBaseURL,
		InstallID:     cfg.ProvisioningInstallID,
		InstallSecret: cfg.ProvisioningInstallSecret,
	}
	orgH := &rest.OrgHandler{
		Store:         st,
		EncryptKeyB64: cfg.SSHEncryptKey,
		Provision:     provH,
	}
	publicH := &rest.PublicHandler{
		Store:                     st,
		ProvisioningBaseURL:       cfg.ProvisioningBaseURL,
		ProvisioningInstallID:     cfg.ProvisioningInstallID,
		ProvisioningInstallSecret: cfg.ProvisioningInstallSecret,
		UpstreamGitHubRepo:        cfg.UpstreamGitHubRepo,
		UpdateManifestURL:         cfg.UpdateManifestURL,
		PublicAppURL:              cfg.PublicAppURL,
	}
	signupReqH := &rest.SignupRequestHandler{Store: st}
	projH := &rest.SpaceHandler{Store: st, Bus: bus}
	taskH := &rest.TaskHandler{Store: st, Bus: bus}
	chatH := &rest.ChatRoomHandler{Store: st}
	chatMsgH := &rest.ChatMessageHandler{Store: st, Bus: bus}
	presenceH := &rest.PresenceHandler{Store: st}
	fileH := &rest.FileNodeHandler{Store: st, OS: objStore, Auth: authSvc, Rdb: rdb, Bus: bus}
	gitH := &rest.SpaceGitHandler{Store: st, OS: objStore, EncryptKeyB64: cfg.SSHEncryptKey, GitWorkdirBase: cfg.GitWorkdirBase}
	previewH := &rest.PreviewHandler{Store: st, OS: objStore, PublicBase: cfg.PublicAPIBaseURL}
	dsH := &rest.DatasetHandler{Store: st, OS: objStore}
	rolesH := &rest.RolesHandler{Store: st}
	notifH := &rest.NotificationsHandler{Store: st}
	peekH := &rest.PeekHandler{Store: st}
	autoH := &rest.AutomationsHandler{Store: st, EncryptKeyB64: cfg.SSHEncryptKey}
	inviteH := &rest.InviteHandler{Store: st}
	saH := &rest.ServiceAccountsHandler{Store: st}
	sapH := &rest.ServiceAccountProfileHandler{Store: st}
	harness := &agenttools.Harness{Store: st, OS: objStore}
	agH := &rest.AgentInvokeHandler{Store: st, Harness: harness}
	propH := &rest.FileProposalHandler{Store: st, OS: objStore}
	sshConnH := &rest.SSHConnectionsHandler{Store: st, EncryptKeyB64: cfg.SSHEncryptKey}
	wsH := &ws.Handler{Auth: authSvc, Store: st, Rdb: rdb}
	termWSH := &terminal.WSHandler{Auth: authSvc, Store: st, EncryptKeyB64: cfg.SSHEncryptKey}
	agentsBaseURL := strings.TrimSpace(cfg.CursorAgentsBaseURL)
	if agentsBaseURL == "" {
		agentsBaseURL = cfg.CursorAPIBaseURL
	}
	orClient := &openrouter.Client{
		BaseURL:    cfg.OpenRouterAPIBaseURL,
		ChatPath:   cfg.OpenRouterChatCompletionsPath,
		HTTPClient: &http.Client{Timeout: 100 * time.Second},
	}
	agentsClient := &agents.Client{
		BaseURL:    agentsBaseURL,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}
	aiWorker := &chatai.Worker{
		Store: st, Rdb: rdb, Bus: bus,
		EncryptKeyB64: cfg.SSHEncryptKey,
		OpenRouter:    orClient,
		Agents:        agentsClient,
		Harness:       harness,
		ORTooling:     chatai.OpenRouterToolingFromConfig(cfg),
		Debug:         cfg.Debug,
	}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go func() {
		if err := aiWorker.Start(workerCtx); err != nil {
			log.Error("chat ai worker", "err", err)
		}
	}()
	overdueW := &overduetasks.Worker{Store: st, Bus: bus}
	go overdueW.Start(workerCtx)

	r := chi.NewRouter()
	r.Use(hsmw.RequestID())
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Logger)
	localDevCORS := strings.Contains(cfg.CORSOrigin, "localhost") || strings.Contains(cfg.CORSOrigin, "127.0.0.1")
	allowOrigin := func(r *http.Request, origin string) bool {
		if cfg.Debug {
			return true
		}
		if localDevCORS {
			return true
		}
		if origin == "" {
			return false
		}
		c := strings.TrimRight(strings.TrimSpace(cfg.CORSOrigin), "/")
		if c != "" && strings.EqualFold(origin, c) {
			return true
		}
		ov, err := st.GetSingletonPublicOriginOverride(r.Context())
		if err != nil || ov == nil {
			return false
		}
		o := strings.TrimRight(strings.TrimSpace(*ov), "/")
		return o != "" && strings.EqualFold(origin, o)
	}
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc:  allowOrigin,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Ensure CORS preflight requests always succeed, even when the router would otherwise return 405.
	r.Options("/*", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"version": version.Version,
			"git_sha": version.GitSHA,
		})
	})

	// Readiness-style healthcheck. Prefer this for Compose/Kubernetes probes.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		dbOK := pool.Ping(ctx) == nil
		redisOK := rdb.Ping(ctx).Err() == nil
		objOK := true
		if objStore != nil && objStore.S3 != nil {
			_, err := objStore.S3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(objStore.Bucket)})
			objOK = err == nil
		}

		status := http.StatusOK
		if !dbOK || !redisOK || !objOK {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(fmt.Sprintf(
			`{"ok":%v,"db":%v,"redis":%v,"object_store":%v,"dual_provider_ai_staff":true}`,
			status == http.StatusOK, dbOK, redisOK, objOK,
		)))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Use(httprate.Limit(30, time.Minute))
			r.Post("/register", authSvc.Register)
			r.Post("/login", authSvc.Login)
			r.Post("/refresh", authSvc.Refresh)
		})

		// WebSocket endpoints authenticate via ?token= (browsers can't set Authorization headers).
		r.Get("/organizations/{orgID}/ws", wsH.ServeWS)
		r.Get("/organizations/{orgID}/spaces/{spaceID}/terminal/ws", termWSH.ServeWS)
		r.Get("/organizations/{orgID}/spaces/{spaceID}/files/collab/ws", fileH.ServeCollabWS)
		// IDE Phase 2 preview static snapshot: iframe loads this URL with ?token= (no Authorization header).
		r.Get("/organizations/{orgID}/spaces/{spaceID}/preview/sessions/{sessionID}/content/*", previewH.ServeContent)

		r.Get("/public/instance", publicH.Instance)

		r.Group(func(r chi.Router) {
			r.Use(hsmw.Auth(authSvc, st))
			r.Get("/me", authSvc.Me)
			r.Patch("/me", authSvc.PatchMe)
			r.Get("/me/notifications", notifH.ListMy)
			r.Post("/me/notifications/mark-read", notifH.MarkReadMy)
			r.Post("/me/notifications/delete", notifH.DeleteMy)
			r.Get("/me/tasks", taskH.ListMine)
			r.Post("/auth/logout", authSvc.Logout)
			r.Post("/invites/{token}/accept", inviteH.Accept)
			r.Post("/presence/ping", presenceH.Ping)
			r.Post("/provisioning/claim", provH.Claim)
			r.Delete("/provisioning/claim/{slug}", provH.DeleteClaim)

			r.Route("/organizations", func(r chi.Router) {
				r.Get("/", orgH.List)
				r.Post("/", orgH.Create)

				r.Route("/{orgID}", func(r chi.Router) {
					r.Use(hsmw.RequireOrgMember(st))
					r.Get("/", orgH.Get)
					r.Patch("/", orgH.Patch)
					r.Get("/members", orgH.Members)
					r.Get("/peek/ai-activity", peekH.AIActivity)
					r.Get("/peek/ai-activity/runs/{replyID}", peekH.AIRunDetail)
					r.Delete("/members/{userID}", orgH.RemoveMember)
					r.Get("/features", orgH.Features)
					r.Patch("/features", orgH.PatchFeatures)
					// Cursor org API key (org.manage). Canonical path + legacy hyphenated alias.
					r.Get("/integrations/cursor", orgH.GetCursorIntegration)
					r.Put("/integrations/cursor", orgH.PutCursorIntegration)
					r.Delete("/integrations/cursor", orgH.DeleteCursorIntegration)
					r.Get("/cursor-integration", orgH.GetCursorIntegration)
					r.Put("/cursor-integration", orgH.PutCursorIntegration)
					r.Delete("/cursor-integration", orgH.DeleteCursorIntegration)
					r.Get("/integrations/openrouter", orgH.GetOpenRouterIntegration)
					r.Put("/integrations/openrouter", orgH.PutOpenRouterIntegration)
					r.Delete("/integrations/openrouter", orgH.DeleteOpenRouterIntegration)
					r.Get("/openrouter-integration", orgH.GetOpenRouterIntegration)
					r.Put("/openrouter-integration", orgH.PutOpenRouterIntegration)
					r.Delete("/openrouter-integration", orgH.DeleteOpenRouterIntegration)
					r.Get("/roles", rolesH.List)
					r.Post("/roles", rolesH.Create)
					r.Patch("/roles/{roleID}", rolesH.Patch)
					r.Delete("/roles/{roleID}", rolesH.Delete)
					r.Get("/members/{userID}/roles", rolesH.MemberRoles)
					r.Put("/members/{userID}/roles", rolesH.SetMemberRoles)
					r.Post("/invites", inviteH.Create)
					r.Get("/signup-requests", signupReqH.List)
					r.Post("/signup-requests/{requestID}/approve", signupReqH.Approve)
					r.Post("/signup-requests/{requestID}/deny", signupReqH.Deny)
					r.Post("/agent-tools/invoke", agH.Invoke)
					r.Get("/agent-tools/tools", agH.ListTools)
					r.Get("/service-accounts", saH.List)
					r.Post("/service-accounts", saH.Create)
					r.Get("/service-accounts/{serviceAccountID}/profile/versions", sapH.Versions)
					r.Get("/service-accounts/{serviceAccountID}/profile", sapH.Get)
					r.Patch("/service-accounts/{serviceAccountID}/profile", sapH.Patch)
					r.Patch("/service-accounts/{serviceAccountID}", saH.Patch)
					r.Delete("/service-accounts/{serviceAccountID}", saH.Delete)
					r.Get("/ssh-connections", sshConnH.List)
					r.Post("/ssh-connections", sshConnH.Create)
					r.Patch("/ssh-connections/{connID}", sshConnH.Patch)
					r.Delete("/ssh-connections/{connID}", sshConnH.Delete)

					r.Route("/spaces", func(r chi.Router) {
						r.Get("/", projH.List)
						r.Post("/", projH.Create)
						r.Get("/access-summary", projH.AccessSummary)
						r.Route("/{spaceID}", func(r chi.Router) {
							// Project access allowlist (org-admin only).
							r.Get("/access", projH.GetAccess)
							r.Put("/access", projH.PutAccess)

							// Everything else requires project membership (or org-level override).
							r.Group(func(r chi.Router) {
								r.Use(hsmw.RequireSpaceMember(st))
								r.Get("/", projH.Get)
								r.Get("/accessible-members", projH.ListAccessibleMembers)
								r.Get("/boards", projH.ListBoards)
								r.Post("/boards", projH.CreateBoard)
								r.Get("/boards/{boardID}", projH.BoardByID)
								r.Delete("/boards/{boardID}", projH.DeleteBoard)
								r.Get("/board", projH.Board)
								r.Get("/chat-rooms", chatH.List)
								r.Post("/chat-rooms", chatH.Create)
								// Register longer /messages/* routes before DELETE /chat-rooms/{id} so paths are unambiguous.
								r.Get("/chat-rooms/{chatRoomID}/messages", chatMsgH.List)
								r.Post("/chat-rooms/{chatRoomID}/messages", chatMsgH.Create)
								r.Patch("/chat-rooms/{chatRoomID}/messages/{messageID}", chatMsgH.Patch)
								r.Delete("/chat-rooms/{chatRoomID}/messages/{messageID}", chatMsgH.Delete)
								r.Post("/chat-rooms/{chatRoomID}/messages/{messageID}/reactions", chatMsgH.AddReaction)
								r.Delete("/chat-rooms/{chatRoomID}/messages/{messageID}/reactions", chatMsgH.RemoveReaction)
								r.Get("/chat-rooms/{chatRoomID}/search", chatMsgH.Search)
								r.Delete("/chat-rooms/{chatRoomID}", chatH.Delete)
								r.Post("/files/proposals/{proposalID}/accept", propH.Accept)
								r.Post("/files/proposals/{proposalID}/reject", propH.Reject)
								r.Get("/files", fileH.List)
								r.Get("/files/tree", fileH.Tree)
								r.Get("/files/{nodeID}", fileH.GetNode)
								r.Post("/files/folders", fileH.CreateFolder)
								r.Post("/files/text", fileH.CreateTextFile)
								r.Post("/files/upload/init", fileH.UploadInit)
								r.Post("/files/upload/complete", fileH.UploadComplete)
								r.Post("/files/export.zip", fileH.ExportZip)
								r.Post("/files/import.zip", fileH.ImportZip)
								r.Patch("/files/{nodeID}", fileH.Patch)
								r.Delete("/files/{nodeID}", fileH.Delete)
								r.Get("/files/{nodeID}/download", fileH.Download)
								r.Put("/files/{nodeID}/upload", fileH.UploadViaAPI)
								r.Get("/files/{nodeID}/text", fileH.GetText)
								r.Put("/files/{nodeID}/text", fileH.PutText)
								r.Get("/files/{nodeID}/proposals", propH.List)
								r.Post("/files/{nodeID}/proposals", propH.Create)

								r.Post("/preview/sessions", previewH.CreateSession)
								r.Get("/preview/sessions/{sessionID}", previewH.GetSession)
								r.Delete("/preview/sessions/{sessionID}", previewH.DeleteSession)

								r.Get("/git", gitH.Get)
								r.Put("/git", gitH.Put)
								r.Delete("/git", gitH.Delete)
								r.Post("/git/test", gitH.Test)
								r.Post("/git/pull", gitH.Pull)
								r.Post("/git/push", gitH.Push)

								r.Get("/datasets", dsH.List)
								r.Post("/datasets/upload/init", dsH.UploadInit)
								r.Post("/datasets/upload/complete", dsH.UploadComplete)
								r.Delete("/datasets/{datasetID}", dsH.Delete)
								r.Get("/datasets/{datasetID}/preview", dsH.Preview)
								r.Post("/datasets/{datasetID}/query", dsH.Query)

								r.Get("/automations", autoH.List)
								r.Post("/automations", autoH.Create)
								r.Patch("/automations/{automationID}", autoH.Patch)
								r.Delete("/automations/{automationID}", autoH.Delete)
								r.Post("/automations/{automationID}/approve", autoH.Approve)
								r.Post("/automations/{automationID}/reject", autoH.Reject)
								r.Post("/automations/{automationID}/run", autoH.Run)
								r.Get("/automations/{automationID}/runs", autoH.ListRuns)

								r.Route("/tasks", func(r chi.Router) {
									r.Get("/", taskH.List)
									r.Post("/", taskH.Create)
									r.Get("/{taskID}/messages", taskH.ListMessages)
									r.Post("/{taskID}/messages", taskH.CreateMessage)
									r.Get("/{taskID}/deliverables", taskH.ListDeliverables)
									r.Post("/{taskID}/deliverables", taskH.LinkDeliverable)
									r.Delete("/{taskID}/deliverables/{fileNodeID}", taskH.UnlinkDeliverable)
									r.Patch("/{taskID}", taskH.Patch)
									r.Delete("/{taskID}", taskH.Delete)
								})

								// Milestones removed.
							})
						})
					})
				})
			})
		})
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if cfg.Debug {
			log.Debug("debug mode on", "cors_origin", cfg.CORSOrigin, "cors_local_dev_permissive", localDevCORS)
		}
		log.Info("listening", "addr", cfg.HTTPAddr, "debug", cfg.Debug)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	workerCancel()
	shctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shctx)
}
