package rest

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"hyperspeed/api/internal/ctxkey"
	"hyperspeed/api/internal/datasetread"
	"hyperspeed/api/internal/files"
	"hyperspeed/api/internal/httpx"
	"hyperspeed/api/internal/middleware"
	"hyperspeed/api/internal/rbac"
	"hyperspeed/api/internal/store"
)

type DatasetHandler struct {
	Store *store.Store
	OS    *files.ObjectStore
}

func (h *DatasetHandler) requireDatasetsEnabled(w http.ResponseWriter, r *http.Request, orgID uuid.UUID) bool {
	f, err := h.Store.OrgFeatures(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "features")
		return false
	}
	if !f.DatasetsEnabled {
		httpx.Error(w, http.StatusForbidden, "datasets are disabled for this organization")
		return false
	}
	return true
}

type datasetUploadInitBody struct {
	Name   string `json:"name"`
	Format string `json:"format"` // parquet | csv
}

func (h *DatasetHandler) UploadInit(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.DatasetsWrite) {
		return
	}
	if !h.requireDatasetsEnabled(w, r, orgID) {
		return
	}
	if isSA, _ := h.Store.ServiceAccountInOrg(r.Context(), orgID, uid); isSA {
		httpx.Error(w, http.StatusForbidden, "service accounts cannot upload datasets directly")
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	var body datasetUploadInitBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name required")
		return
	}
	var fmtEnum store.DatasetFormat
	switch strings.ToLower(strings.TrimSpace(body.Format)) {
	case "parquet":
		fmtEnum = store.DatasetFormatParquet
	case "csv":
		fmtEnum = store.DatasetFormatCSV
	default:
		httpx.Error(w, http.StatusBadRequest, "format must be parquet or csv")
		return
	}
	ext := string(fmtEnum)
	storageKey := "org/" + orgID.String() + "/spaces/" + spaceID.String() + "/datasets/" + uuid.NewString() + "." + ext
	ds, err := h.Store.CreateDatasetPending(r.Context(), orgID, spaceID, body.Name, fmtEnum, storageKey, uid)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			httpx.Error(w, http.StatusConflict, "dataset name already exists in this space")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "create dataset")
		return
	}
	ct := "application/octet-stream"
	if fmtEnum == store.DatasetFormatCSV {
		ct = "text/csv"
	}
	uploadURL, err := h.OS.PresignPut(r.Context(), storageKey, ct, 15*time.Minute)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "presign upload")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"dataset":    ds,
		"upload_url": uploadURL,
	})
}

type datasetCompleteBody struct {
	DatasetID uuid.UUID `json:"dataset_id"`
}

func (h *DatasetHandler) UploadComplete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.DatasetsWrite) {
		return
	}
	if !h.requireDatasetsEnabled(w, r, orgID) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	if h.OS == nil {
		httpx.Error(w, http.StatusInternalServerError, "object store not configured")
		return
	}
	var body datasetCompleteBody
	if err := httpx.DecodeJSON(r, &body); err != nil || body.DatasetID == uuid.Nil {
		httpx.Error(w, http.StatusBadRequest, "dataset_id required")
		return
	}
	ds, err := h.Store.DatasetByID(r.Context(), spaceID, body.DatasetID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "dataset not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "dataset")
		return
	}
	start := time.Now()
	size, err := h.OS.HeadSize(r.Context(), ds.StorageKey)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "object not found or not uploaded yet")
		return
	}
	if size <= 0 {
		httpx.Error(w, http.StatusBadRequest, "empty upload")
		return
	}
	if size > datasetread.MaxProcessBytes {
		httpx.Error(w, http.StatusBadRequest, "file too large for server-side processing")
		return
	}
	tmp, err := os.CreateTemp("", "hs-dataset-*")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "temp file")
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := h.OS.DownloadToPath(r.Context(), ds.StorageKey, tmpPath, datasetread.MaxProcessBytes); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "download object")
		return
	}
	var schema datasetread.Schema
	var rowCount *int64
	switch ds.Format {
	case store.DatasetFormatParquet:
		sch, n, err := datasetread.InferParquet(tmpPath)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid parquet: "+err.Error())
			return
		}
		schema = sch
		rowCount = &n
	case store.DatasetFormatCSV:
		sch, n, err := datasetread.InferCSV(tmpPath, datasetread.MaxProcessBytes)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid csv: "+err.Error())
			return
		}
		schema = sch
		rowCount = &n
	default:
		httpx.Error(w, http.StatusBadRequest, "unsupported format")
		return
	}
	schemaJSON, err := datasetread.SchemaJSON(schema)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "schema")
		return
	}
	out, err := h.Store.CompleteDataset(r.Context(), spaceID, body.DatasetID, size, schemaJSON, rowCount)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "complete dataset")
		return
	}
	_ = h.Store.LogDatasetQueryInvocation(r.Context(), orgID, spaceID, body.DatasetID, uid, "upload_complete", nil, 0, int(time.Since(start).Milliseconds()))
	httpx.JSON(w, http.StatusOK, map[string]any{"dataset": out})
}

func (h *DatasetHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.DatasetsRead) {
		return
	}
	if !h.requireDatasetsEnabled(w, r, orgID) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	list, err := h.Store.ListDatasets(r.Context(), spaceID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "list datasets")
		return
	}
	if list == nil {
		list = []store.SpaceDataset{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"datasets": list})
}

func (h *DatasetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.DatasetsWrite) {
		return
	}
	if !h.requireDatasetsEnabled(w, r, orgID) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	datasetID, err := uuid.Parse(chi.URLParam(r, "datasetID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid dataset id")
		return
	}
	ds, err := h.Store.DatasetByID(r.Context(), spaceID, datasetID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "dataset not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "dataset")
		return
	}
	if h.OS != nil && ds.StorageKey != "" {
		_ = h.OS.Delete(r.Context(), ds.StorageKey)
	}
	if err := h.Store.DeleteDataset(r.Context(), spaceID, datasetID); err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "dataset not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *DatasetHandler) Preview(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.DatasetsRead) {
		return
	}
	if !h.requireDatasetsEnabled(w, r, orgID) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	datasetID, err := uuid.Parse(chi.URLParam(r, "datasetID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid dataset id")
		return
	}
	ds, err := h.Store.DatasetByID(r.Context(), spaceID, datasetID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "dataset not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "dataset")
		return
	}
	if ds.SizeBytes == nil || *ds.SizeBytes <= 0 {
		httpx.Error(w, http.StatusBadRequest, "dataset upload not completed")
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			limit = n
		}
	}
	start := time.Now()
	tmp, err := os.CreateTemp("", "hs-dataset-preview-*")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "temp file")
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := h.OS.DownloadToPath(r.Context(), ds.StorageKey, tmpPath, datasetread.MaxProcessBytes); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "download")
		return
	}
	var table datasetread.TableResult
	switch ds.Format {
	case store.DatasetFormatParquet:
		table, err = datasetread.PreviewParquet(tmpPath, limit)
	case store.DatasetFormatCSV:
		table, err = datasetread.PreviewCSV(tmpPath, limit, datasetread.MaxProcessBytes)
	default:
		httpx.Error(w, http.StatusBadRequest, "unsupported format")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	reqLog, _ := json.Marshal(map[string]any{"limit": limit})
	_ = h.Store.LogDatasetQueryInvocation(r.Context(), orgID, spaceID, datasetID, uid, "preview", reqLog, len(table.Rows), int(time.Since(start).Milliseconds()))
	httpx.JSON(w, http.StatusOK, map[string]any{"preview": table})
}

func (h *DatasetHandler) Query(w http.ResponseWriter, r *http.Request) {
	orgID, _ := middleware.OrgIDFromContext(r.Context())
	uid, ok := ctxkey.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !requireOrgPerm(w, r, h.Store, orgID, uid, rbac.DatasetsRead) {
		return
	}
	if !h.requireDatasetsEnabled(w, r, orgID) {
		return
	}
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid space id")
		return
	}
	if _, err := h.Store.GetSpace(r.Context(), orgID, spaceID); err != nil {
		httpx.Error(w, http.StatusNotFound, "space not found")
		return
	}
	datasetID, err := uuid.Parse(chi.URLParam(r, "datasetID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid dataset id")
		return
	}
	ds, err := h.Store.DatasetByID(r.Context(), spaceID, datasetID)
	if err != nil {
		if err == pgx.ErrNoRows {
			httpx.Error(w, http.StatusNotFound, "dataset not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "dataset")
		return
	}
	if ds.SizeBytes == nil || *ds.SizeBytes <= 0 {
		httpx.Error(w, http.StatusBadRequest, "dataset upload not completed")
		return
	}
	var reqBody datasetread.QueryRequest
	if err := httpx.DecodeJSON(r, &reqBody); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if reqBody.Limit <= 0 {
		reqBody.Limit = datasetread.MaxQueryRows
	}
	start := time.Now()
	tmp, err := os.CreateTemp("", "hs-dataset-query-*")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "temp file")
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := h.OS.DownloadToPath(r.Context(), ds.StorageKey, tmpPath, datasetread.MaxProcessBytes); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "download")
		return
	}
	var table datasetread.TableResult
	switch ds.Format {
	case store.DatasetFormatParquet:
		table, err = datasetread.QueryParquet(tmpPath, reqBody)
	case store.DatasetFormatCSV:
		table, err = datasetread.QueryCSV(tmpPath, reqBody, datasetread.MaxProcessBytes)
	default:
		httpx.Error(w, http.StatusBadRequest, "unsupported format")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	reqLog, _ := json.Marshal(reqBody)
	_ = h.Store.LogDatasetQueryInvocation(r.Context(), orgID, spaceID, datasetID, uid, "query", reqLog, len(table.Rows), int(time.Since(start).Milliseconds()))
	httpx.JSON(w, http.StatusOK, map[string]any{"result": table})
}
