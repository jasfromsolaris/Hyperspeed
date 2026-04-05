package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type TaskHandler struct {
	Store *store.Store
	Bus   *events.Bus
}

func (h *TaskHandler) validateAssignee(ctx context.Context, orgID uuid.UUID, assignee *uuid.UUID) error {
	if assignee == nil || *assignee == uuid.Nil {
		return nil
	}
	_, err := h.Store.MemberRole(ctx, orgID, *assignee)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidAssignee
		}
		return err
	}
	return nil
}

var errInvalidAssignee = errors.New("invalid assignee")

func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksRead) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	tasks, err := h.Store.ListTasksByProject(r.Context(), pid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list tasks")
		return
	}
	if tasks == nil {
		tasks = []store.Task{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

type createTaskBody struct {
	Title                   string     `json:"title"`
	Description             string     `json:"description"`
	ColumnID                uuid.UUID  `json:"column_id"`
	DueAt                   *time.Time `json:"due_at"`
	AssigneeUserID          *uuid.UUID `json:"assignee_user_id"`
	DeliverableRequired     *bool   `json:"deliverable_required"`
	DeliverableInstructions string  `json:"deliverable_instructions"`
}

func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	p, err := h.Store.GetSpace(r.Context(), orgID, pid)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	var body createTaskBody
	if err := httpx.DecodeJSONLenient(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Title == "" || body.ColumnID == uuid.Nil {
		httpx.Error(w, http.StatusBadRequest, "title and column_id required")
		return
	}
	if err := h.validateAssignee(r.Context(), orgID, body.AssigneeUserID); err != nil {
		if errors.Is(err, errInvalidAssignee) {
			httpx.Error(w, http.StatusBadRequest, "invalid assignee")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "assignee")
		return
	}
	ok, err := h.Store.VerifyColumnBelongsToProject(r.Context(), p.ID, body.ColumnID)
	if err != nil || !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid column")
		return
	}
	boardID, err := h.Store.GetBoardIDForColumnInProject(r.Context(), p.ID, body.ColumnID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusBadRequest, "invalid column")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "board")
		return
	}
	pos, err := h.Store.MaxTaskPositionInColumn(r.Context(), body.ColumnID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "position")
		return
	}
	deliverReq := body.DeliverableRequired != nil && *body.DeliverableRequired
	if deliverReq {
		_, err := h.Store.EnsureDeliverablesFolder(r.Context(), p.ID, uid)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "deliverables folder")
			return
		}
	}
	t, err := h.Store.CreateTask(r.Context(), p.ID, boardID, body.ColumnID, pos+1, store.CreateTaskParams{
		Title:                   body.Title,
		Description:             body.Description,
		AssigneeUserID:          body.AssigneeUserID,
		DeliverableRequired:     deliverReq,
		DeliverableInstructions: body.DeliverableInstructions,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "create task")
		return
	}
	if body.DueAt != nil {
		t, err = h.Store.UpdateTask(r.Context(), p.ID, t.ID, store.TaskPatch{DueAt: body.DueAt})
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "due date")
			return
		}
	}
	h.publishTask(orgID, p.ID, events.TaskCreated, t)
	h.maybeNotifyAssignee(r.Context(), orgID, p.ID, uid, nil, &t)
	httpx.JSON(w, http.StatusCreated, t)
}

type patchTaskBody struct {
	Title                     *string    `json:"title"`
	Description               *string    `json:"description"`
	ColumnID                  *uuid.UUID `json:"column_id"`
	Position                  *int       `json:"position"`
	AssigneeUserID            *uuid.UUID `json:"assignee_user_id"`
	DueAt                     *time.Time `json:"due_at"`
	ClearDueAt                *bool      `json:"clear_due_at"`
	DeliverableRequired       *bool      `json:"deliverable_required"`
	DeliverableInstructions   *string    `json:"deliverable_instructions"`
}

func (h *TaskHandler) Patch(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	tid, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	prev, err := h.Store.GetTask(r.Context(), pid, tid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "task not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "task")
		return
	}
	var body patchTaskBody
	if err := httpx.DecodeJSONLenient(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.AssigneeUserID != nil {
		if err := h.validateAssignee(r.Context(), orgID, body.AssigneeUserID); err != nil {
			if errors.Is(err, errInvalidAssignee) {
				httpx.Error(w, http.StatusBadRequest, "invalid assignee")
				return
			}
			httpx.Error(w, http.StatusInternalServerError, "assignee")
			return
		}
	}
	if body.ColumnID != nil {
		ok, err := h.Store.VerifyColumnBelongsToProject(r.Context(), pid, *body.ColumnID)
		if err != nil || !ok {
			httpx.Error(w, http.StatusBadRequest, "invalid column")
			return
		}
	}
	wantDeliver := prev.DeliverableRequired
	if body.DeliverableRequired != nil {
		wantDeliver = *body.DeliverableRequired
	}
	if wantDeliver && !prev.DeliverableRequired {
		_, err := h.Store.EnsureDeliverablesFolder(r.Context(), pid, uid)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "deliverables folder")
			return
		}
	}
	patch := store.TaskPatch{
		Title:                   body.Title,
		Description:             body.Description,
		ColumnID:                body.ColumnID,
		Position:                body.Position,
		AssigneeUserID:          body.AssigneeUserID,
		DueAt:                   body.DueAt,
		ClearDueAt:              body.ClearDueAt,
		DeliverableRequired:     body.DeliverableRequired,
		DeliverableInstructions: body.DeliverableInstructions,
	}
	t, err := h.Store.UpdateTask(r.Context(), pid, tid, patch)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "task not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "update task")
		return
	}
	h.publishTask(orgID, pid, events.TaskUpdated, t)
	h.maybeNotifyAssignee(r.Context(), orgID, pid, uid, &prev, &t)
	httpx.JSON(w, http.StatusOK, t)
}

func (h *TaskHandler) maybeNotifyAssignee(ctx context.Context, orgID, spaceID, actor uuid.UUID, prev, cur *store.Task) {
	if cur == nil {
		return
	}
	var prevAssign *uuid.UUID
	if prev != nil {
		prevAssign = prev.AssigneeUserID
	}
	newAssign := cur.AssigneeUserID
	if newAssign == nil || *newAssign == uuid.Nil {
		return
	}
	if prevAssign != nil && newAssign != nil && *prevAssign == *newAssign {
		return
	}
	payload := map[string]any{
		"organization_id":     orgID.String(),
		"space_id":            spaceID.String(),
		"board_id":            cur.BoardID.String(),
		"task_id":             cur.ID.String(),
		"title":               cur.Title,
		"assigned_by_user_id": actor.String(),
		"task_version":        cur.Version,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	n, err := h.Store.CreateNotification(ctx, orgID, *newAssign, "task.assigned", b)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return
		}
		return
	}
	if h.Bus != nil {
		pid := spaceID
		payloadN, err := events.Marshal(events.NotificationCreated, orgID, &pid, map[string]any{
			"user_id":      newAssign.String(),
			"notification": n,
		})
		if err == nil {
			_ = h.Bus.Publish(context.Background(), orgID, payloadN)
		}
	}
}

func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	tid, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if err := h.Store.DeleteTask(r.Context(), pid, tid); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "task not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "delete")
		return
	}
	h.publishTaskDelete(orgID, pid, tid)
	w.WriteHeader(http.StatusNoContent)
}

func (h *TaskHandler) publishTask(orgID, projectID uuid.UUID, typ events.Type, t store.Task) {
	if h.Bus == nil {
		return
	}
	pid := projectID
	payload, err := events.Marshal(typ, orgID, &pid, t)
	if err != nil {
		return
	}
	_ = h.Bus.Publish(context.Background(), orgID, payload)
}

func (h *TaskHandler) publishTaskDelete(orgID, projectID, taskID uuid.UUID) {
	if h.Bus == nil {
		return
	}
	pid := projectID
	payload, err := events.Marshal(events.TaskDeleted, orgID, &pid, map[string]string{"id": taskID.String()})
	if err != nil {
		return
	}
	_ = h.Bus.Publish(context.Background(), orgID, payload)
}

// ListMine returns tasks assigned to the current user for an organization (GET /me/tasks?org_id=).
func (h *TaskHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.URL.Query().Get("org_id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil || orgID == uuid.Nil {
		httpx.Error(w, http.StatusBadRequest, "org_id required")
		return
	}
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksRead) {
		return
	}
	rows, err := h.Store.ListMyAssignedTasksInOrg(r.Context(), orgID, uid, 200)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list tasks")
		return
	}
	out := make([]store.MyAssignedTaskRow, 0, len(rows))
	for _, row := range rows {
		ok, err := h.Store.UserHasEffectiveSpaceAccess(r.Context(), orgID, row.ProjectID, uid)
		if err != nil || !ok {
			continue
		}
		out = append(out, row)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tasks": out})
}

// ListMessages GET .../tasks/{taskID}/messages
func (h *TaskHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksRead) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	tid, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if _, err := h.Store.GetTask(r.Context(), pid, tid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "task not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "task")
		return
	}
	limit := 50
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			limit = n
		}
	}
	var before *time.Time
	if bs := r.URL.Query().Get("before"); bs != "" {
		tm, err := time.Parse(time.RFC3339Nano, bs)
		if err == nil {
			before = &tm
		}
	}
	msgs, err := h.Store.ListTaskMessages(r.Context(), pid, tid, limit, before)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "messages")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

type postTaskMessageBody struct {
	Content string `json:"content"`
}

// CreateMessage POST .../tasks/{taskID}/messages
func (h *TaskHandler) CreateMessage(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	tid, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if _, err := h.Store.GetTask(r.Context(), pid, tid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "task not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "task")
		return
	}
	var body postTaskMessageBody
	if err := httpx.DecodeJSONLenient(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Content == "" {
		httpx.Error(w, http.StatusBadRequest, "content required")
		return
	}
	m, err := h.Store.InsertTaskMessage(r.Context(), pid, tid, uid, body.Content)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "message")
		return
	}
	if h.Bus != nil {
		pidCopy := pid
		payload, err := events.Marshal(events.TaskMessageCreated, orgID, &pidCopy, m)
		if err == nil {
			_ = h.Bus.Publish(r.Context(), orgID, payload)
		}
	}
	httpx.JSON(w, http.StatusCreated, m)
}

// ListDeliverables GET .../tasks/{taskID}/deliverables
func (h *TaskHandler) ListDeliverables(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksRead) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	tid, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if _, err := h.Store.GetTask(r.Context(), pid, tid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "task not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "task")
		return
	}
	list, err := h.Store.ListTaskDeliverableFiles(r.Context(), pid, tid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "deliverables")
		return
	}
	if list == nil {
		list = []store.TaskDeliverableFile{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"deliverables": list})
}

type linkDeliverableBody struct {
	FileNodeID uuid.UUID `json:"file_node_id"`
}

// LinkDeliverable POST .../tasks/{taskID}/deliverables
func (h *TaskHandler) LinkDeliverable(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	tid, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if _, err := h.Store.GetTask(r.Context(), pid, tid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "task not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "task")
		return
	}
	var body linkDeliverableBody
	if err := httpx.DecodeJSONLenient(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.FileNodeID == uuid.Nil {
		httpx.Error(w, http.StatusBadRequest, "file_node_id required")
		return
	}
	folder, err := h.Store.EnsureDeliverablesFolder(r.Context(), pid, uid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "deliverables folder")
		return
	}
	fn, err := h.Store.FileNodeByID(r.Context(), pid, body.FileNodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "file not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "file")
		return
	}
	if fn.Kind != store.FileNodeFile || fn.DeletedAt != nil {
		httpx.Error(w, http.StatusBadRequest, "not a file")
		return
	}
	ok, err := h.Store.FileIsUnderFolder(r.Context(), pid, body.FileNodeID, folder.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "file path")
		return
	}
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "file must be under the Deliverables folder")
		return
	}
	if err := h.Store.LinkTaskDeliverableFile(r.Context(), pid, tid, body.FileNodeID, uid); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "link")
		return
	}
	t, err := h.Store.GetTask(r.Context(), pid, tid)
	if err == nil {
		h.publishTask(orgID, pid, events.TaskUpdated, t)
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnlinkDeliverable DELETE .../tasks/{taskID}/deliverables/{fileNodeID}
func (h *TaskHandler) UnlinkDeliverable(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, hasUser := ctxkey.UserID(r.Context())
	if !hasUser {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.TasksWrite) {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	tid, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid task id")
		return
	}
	fid, err := uuid.Parse(chi.URLParam(r, "fileNodeID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid file id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, pid); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if _, err := h.Store.GetTask(r.Context(), pid, tid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "task not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "task")
		return
	}
	ok, err := h.Store.UnlinkTaskDeliverableFile(r.Context(), tid, fid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "unlink")
		return
	}
	if !ok {
		httpx.Error(w, http.StatusNotFound, "not linked")
		return
	}
	t, err := h.Store.GetTask(r.Context(), pid, tid)
	if err == nil {
		h.publishTask(orgID, pid, events.TaskUpdated, t)
	}
	w.WriteHeader(http.StatusNoContent)
}
