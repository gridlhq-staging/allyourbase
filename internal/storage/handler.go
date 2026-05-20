// Package storage Handler serves HTTP endpoints for file storage operations including upload, download, deletion, signing, and listing, with support for resumable uploads via TUS protocol, image transformations, and tenant quota enforcement.
package storage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc           *Service
	isAdmin       func(*http.Request) bool
	logger        *slog.Logger
	maxFileSize   int64
	cdnURL        string
	uploadTimeout time.Duration

	mutations           handlerMutations
	cdnPurgeCoordinator *cdnPurgeCoordinator

	tenantQuotaReader      tenantQuotaReader
	tenantQuotaChecker     tenant.QuotaChecker
	tenantUsageAccumulator *tenant.UsageAccumulator
	tenantQuotaMetrics     tenantQuotaMetricsRecorder
	tenantQuotaAudit       tenantQuotaAuditEmitter
}

const (
	headerTenantQuotaWarning = "X-Tenant-Quota-Warning"

	tusResumableVersion     = "1.0.0"
	tusResumableExtension   = "creation"
	tusResumableHeader      = "Tus-Resumable"
	tusVersionHeader        = "Tus-Version"
	tusExtensionHeader      = "Tus-Extension"
	tusMaxSizeHeader        = "Tus-Max-Size"
	tusUploadLengthHeader   = "Upload-Length"
	tusUploadOffsetHeader   = "Upload-Offset"
	tusUploadMetadataHeader = "Upload-Metadata"
	tusOffsetContentType    = "application/offset+octet-stream"

	defaultUploadTimeout = 5 * time.Minute
)

func NewHandler(svc *Service, logger *slog.Logger, maxFileSize int64, cdnURL string, isAdmin ...func(*http.Request) bool) *Handler {
	var isAdminFn func(*http.Request) bool
	if len(isAdmin) > 0 {
		isAdminFn = isAdmin[0]
	}
	return &Handler{
		svc:           svc,
		isAdmin:       isAdminFn,
		logger:        logger,
		maxFileSize:   maxFileSize,
		cdnURL:        strings.TrimSpace(cdnURL),
		uploadTimeout: defaultUploadTimeout,
		mutations:     newHandlerMutations(svc),
		cdnPurgeCoordinator: newCDNPurgeCoordinator(
			NopCDNProvider{},
			logger,
			defaultCDNPurgeTimeout,
		),
	}
}

// SetUploadTimeout overrides the server-side upload timeout used for backend
// writes. A non-positive value disables the handler-level timeout.
func (h *Handler) SetUploadTimeout(d time.Duration) {
	h.uploadTimeout = d
}

// Routes returns a chi.Router with storage endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Route("/upload/resumable", func(r chi.Router) {
		r.Options("/", h.HandleResumableOptions)
		r.Post("/", h.HandleResumableCreate)
		r.Head("/{id}", h.HandleResumableHead)
		r.Patch("/{id}", h.HandleResumablePatch)
	})
	r.Get("/{bucket}", h.HandleList)
	r.Post("/{bucket}", h.HandleUpload)
	r.Get("/{bucket}/*", h.HandleServe)
	r.Delete("/{bucket}/*", h.HandleDelete)
	r.Post("/{bucket}/{name}/sign", h.HandleSign)
	return r
}

type listResponse struct {
	Items      []listItemResponse `json:"items"`
	TotalItems int                `json:"totalItems"`
}

type listItemResponse struct {
	Object
	URL string `json:"url,omitempty"`
}

// HandleList returns a paginated list of objects in a bucket with optional prefix filtering. Each item includes a public URL if the bucket is public, or an empty URL if access is restricted.
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	prefix := r.URL.Query().Get("prefix")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	objects, total, err := h.svc.ListObjects(r.Context(), bucket, prefix, limit, offset)
	if err != nil {
		if errors.Is(err, ErrInvalidBucket) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("list error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if objects == nil {
		objects = []Object{}
	}

	isPublic, err := h.isBucketPublic(r.Context(), bucket)
	if err != nil {
		h.logger.Error("checking bucket access", "error", err)
		isPublic = false
	}

	items := make([]listItemResponse, 0, len(objects))
	for _, obj := range objects {
		item := listItemResponse{Object: obj}
		item.URL = h.publicObjectResponseURL(r, obj, isPublic)
		items = append(items, item)
	}
	httputil.WriteJSON(w, http.StatusOK, listResponse{Items: items, TotalItems: total})
}

// HandleServe serves a file from storage, first checking for a valid signed URL signature. If present and valid, the file is served without authentication. Otherwise, the bucket's public status is checked; private buckets require authentication while public buckets are accessible to all.
func (h *Handler) HandleServe(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	name := chi.URLParam(r, "*")

	// Check for signed URL params.
	if sig := r.URL.Query().Get("sig"); sig != "" {
		exp := r.URL.Query().Get("exp")
		if !h.svc.ValidateSignedURL(bucket, name, exp, sig) {
			httputil.WriteErrorWithDocURL(w, http.StatusForbidden, "invalid or expired signed URL",
				"https://allyourbase.io/guide/file-storage")
			return
		}
		// Signed URL is valid — serve the file without further auth checks.
		// Treat signed URLs as private to avoid cache leakage.
		h.serveFile(w, r, bucket, name, false)
		return
	}

	isPublic, err := h.isBucketPublic(r.Context(), bucket)
	if err != nil {
		h.logger.Error("checking bucket access", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !isPublic {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil && !h.isAdminToken(r) {
			httputil.WriteError(w, http.StatusUnauthorized, "missing auth token")
			return
		}
	}

	h.serveFile(w, r, bucket, name, isPublic)
}

// isBucketPublic determines whether a bucket allows public access. Without a database pool it returns true for backward compatibility. If a bucket has no metadata record, it is treated as implicitly public.
func (h *Handler) isBucketPublic(ctx context.Context, bucket string) (bool, error) {
	// Without a DB pool, preserve backward compatibility by allowing access
	// and keeping historical behavior (public by default).
	if h.svc.pool == nil {
		return true, nil
	}

	b, err := h.svc.GetBucket(ctx, bucket)
	if err != nil {
		if errors.Is(err, ErrBucketNotFound) {
			// Buckets without metadata are treated as implicitly public.
			return true, nil
		}
		return false, err
	}

	return b.Public, nil
}

func (h *Handler) isAdminToken(r *http.Request) bool {
	if h.isAdmin == nil {
		return false
	}
	return h.isAdmin(r)
}

// callerUserID returns the authenticated user's ID for ownership checks.
// Returns nil for admin requests (bypasses ownership) or unauthenticated requests.
func (h *Handler) callerUserID(r *http.Request) *string {
	if h.isAdminToken(r) {
		return nil // admin bypass
	}
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		return &claims.Subject
	}
	return nil
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, bucket, name string, isPublic bool) {
	reader, obj, err := h.svc.Download(r.Context(), bucket, name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("download error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer reader.Close()

	// If image transform query params are present, process and serve transformed image.
	if hasTransformParams(r) {
		h.serveTransformed(w, r, reader, obj, isPublic)
		return
	}

	if applyConditionalRawETag(w, r, obj) {
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	w.Header().Set("Cache-Control", cacheControlRaw(isPublic))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

// hasTransformParams returns true if the request contains image transform query parameters.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	name := chi.URLParam(r, "*")
	tenantID := tenant.TenantFromContext(r.Context())

	// Fetch object metadata before deletion for usage accounting.
	obj, err := h.mutations.getObject(r.Context(), bucket, name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("get object for delete error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.mutations.deleteObject(r.Context(), bucket, name); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		if errors.Is(err, ErrPermissionDenied) {
			httputil.WriteError(w, http.StatusForbidden, "insufficient storage permissions")
			return
		}
		h.logger.Error("delete error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Reclaim quota after successful deletion.
	if obj.UserID != nil {
		if err := h.mutations.decrementUsage(r.Context(), *obj.UserID, obj.Size); err != nil {
			h.logger.Error("decrement usage after delete", "error", err)
			// Non-fatal: file deleted successfully; log the accounting failure and continue.
		}
	}
	if tenantID != "" && h.tenantUsageAccumulator != nil {
		h.tenantUsageAccumulator.Record(tenantID, tenant.ResourceTypeDBSizeBytes, -obj.Size)
	}
	h.enqueueObjectURLPurge(r, bucket, name)

	w.WriteHeader(http.StatusNoContent)
}
