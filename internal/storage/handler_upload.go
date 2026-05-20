// Package storage contains multipart upload request handling helpers.
package storage

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

type uploadResponse struct {
	Object
	URL string `json:"url,omitempty"`
}

type uploadRequestInput struct {
	bucket      string
	name        string
	contentType string
	userID      *string
	trackedUser bool
	file        multipart.File
	size        int64
}

func (h *Handler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	input, ok := h.parseUploadRequest(w, r)
	if !ok {
		return
	}
	defer input.file.Close()

	tenantID := tenant.TenantFromContext(r.Context())
	if tenantID != "" {
		softWarning, currentUsage, limit, err := h.applyTenantQuotaChecks(r.Context(), tenantID, input.size)
		if err != nil {
			if errors.Is(err, ErrQuotaExceeded) {
				h.emitTenantStorageQuotaViolation(r, tenantID, currentUsage, limit)
				httputil.WriteError(w, http.StatusRequestEntityTooLarge, "tenant storage quota exceeded")
			} else {
				h.logger.Error("tenant storage quota check failed", "tenant_id", tenantID, "error", err)
				httputil.WriteError(w, http.StatusInternalServerError, "tenant storage quota check is temporarily unavailable")
			}
			return
		}
		if softWarning {
			w.Header().Set(headerTenantQuotaWarning, "storage")
		}
	}

	reservedBytes := int64(0)
	if input.trackedUser {
		if err := h.mutations.reserveQuota(r.Context(), *input.userID, input.size); err != nil {
			if errors.Is(err, ErrQuotaExceeded) {
				httputil.WriteError(w, http.StatusRequestEntityTooLarge, "storage quota exceeded")
				return
			}
			h.logger.Error("quota reservation error", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		reservedBytes = input.size
	}

	obj, err, timedOut := h.uploadWithTimeout(
		r.Context(),
		input.bucket,
		input.name,
		input.contentType,
		input.userID,
		input.file,
	)
	if err != nil {
		h.rollbackReservedQuota(r.Context(), input.userID, reservedBytes, input.trackedUser)
		if timedOut {
			h.logger.Warn("upload timed out", "timeout", h.uploadTimeout, "bucket", input.bucket, "name", input.name)
			httputil.WriteError(w, http.StatusGatewayTimeout, "upload timed out")
			return
		}
		if errors.Is(err, ErrInvalidBucket) || errors.Is(err, ErrInvalidName) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("upload error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if tenantID != "" && h.tenantUsageAccumulator != nil {
		h.tenantUsageAccumulator.Record(tenantID, tenant.ResourceTypeDBSizeBytes, obj.Size)
	}

	isPublic, publicErr := h.isBucketPublic(r.Context(), input.bucket)
	if publicErr != nil {
		h.logger.Error("checking bucket access", "error", publicErr)
		isPublic = false
	}
	if objectWasOverwritten(obj) {
		h.enqueueObjectURLPurge(r, obj.Bucket, obj.Name)
	}

	resp := uploadResponse{Object: *obj}
	resp.URL = h.publicObjectResponseURL(r, *obj, isPublic)
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) parseUploadRequest(w http.ResponseWriter, r *http.Request) (*uploadRequestInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize)
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid multipart form or file too large",
			"https://allyourbase.io/guide/file-storage")
		return nil, false
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "missing \"file\" field in multipart form",
			"https://allyourbase.io/guide/file-storage")
		return nil, false
	}

	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}
	if name == "" {
		file.Close()
		httputil.WriteError(w, http.StatusBadRequest, "file name is required")
		return nil, false
	}

	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = header.Header.Get("Content-Type")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var userID *string
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		userID = &claims.Subject
	}

	return &uploadRequestInput{
		bucket:      chi.URLParam(r, "bucket"),
		name:        name,
		contentType: contentType,
		userID:      userID,
		trackedUser: userID != nil && !h.isAdminToken(r),
		file:        file,
		size:        header.Size,
	}, true
}

func (h *Handler) uploadWithTimeout(
	requestCtx context.Context,
	bucket, name, contentType string,
	userID *string,
	file io.Reader,
) (*Object, error, bool) {
	uploadCtx := requestCtx
	cancel := func() {}
	if h.uploadTimeout > 0 {
		uploadCtx, cancel = context.WithTimeout(requestCtx, h.uploadTimeout)
	}
	defer cancel()

	obj, err := h.mutations.upload(uploadCtx, bucket, name, contentType, userID, file)
	timedOut := err != nil &&
		errors.Is(err, context.DeadlineExceeded) &&
		requestCtx.Err() == nil &&
		uploadCtx.Err() == context.DeadlineExceeded
	return obj, err, timedOut
}

func (h *Handler) rollbackReservedQuota(
	ctx context.Context,
	userID *string,
	reservedBytes int64,
	trackedUser bool,
) {
	if !trackedUser || reservedBytes <= 0 || userID == nil {
		return
	}
	if rollbackErr := h.mutations.decrementUsage(ctx, *userID, reservedBytes); rollbackErr != nil {
		h.logger.Error("quota rollback error", "error", rollbackErr)
	}
}
