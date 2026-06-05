package allyourbase

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func mustLoadContractFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "tests", "contract", "fixtures", "sdk_contract", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestContractMagicLinkFixturesDecodeThroughModelOwners(t *testing.T) {
	reqData := mustLoadContractFixture(t, "magic_link_request_response.json")
	var reqRes MagicLinkRequestResponse
	if err := json.Unmarshal(reqData, &reqRes); err != nil {
		t.Fatalf("decode magic_link_request_response: %v", err)
	}
	if reqRes.Message != "If an account exists, a magic link has been sent." {
		t.Fatalf("unexpected message %q", reqRes.Message)
	}

	confirmData := mustLoadContractFixture(t, "magic_link_confirm_success_response.json")
	var confirm MagicLinkConfirmResponse
	if err := json.Unmarshal(confirmData, &confirm); err != nil {
		t.Fatalf("decode magic_link_confirm_success_response: %v", err)
	}
	if confirm.Auth == nil {
		t.Fatalf("expected Auth != nil for success fixture")
	}
	if confirm.Auth.User.Email != "magic@allyourbase.io" {
		t.Fatalf("unexpected email %q", confirm.Auth.User.Email)
	}
	if confirm.Auth.User.EmailVerified == nil || !*confirm.Auth.User.EmailVerified {
		t.Fatalf("expected EmailVerified=true, got %+v", confirm.Auth.User.EmailVerified)
	}
	if confirm.Auth.User.CreatedAt != "2026-05-01T12:00:00Z" {
		t.Fatalf("unexpected created_at %q", confirm.Auth.User.CreatedAt)
	}
	if confirm.Auth.User.UpdatedAt != nil {
		t.Fatalf("expected nil updated_at, got %+v", confirm.Auth.User.UpdatedAt)
	}
}

func TestContractPendingMFAFixtureDecodeThroughModelOwner(t *testing.T) {
	data := mustLoadContractFixture(t, "magic_link_confirm_pending_mfa_response.json")
	var out MagicLinkConfirmResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode magic_link_confirm_pending_mfa_response: %v", err)
	}
	if !out.MFAPending {
		t.Fatalf("expected MFAPending=true, got %+v", out)
	}
	if out.MFAToken != "mfa_pending_token_stage1" {
		t.Fatalf("unexpected MFA token %q", out.MFAToken)
	}
	if out.Auth != nil {
		t.Fatalf("expected Auth=nil for pending-MFA fixture, got %+v", out.Auth)
	}
}

func TestContractAnonymousAndLinkEmailUserShapeNormalization(t *testing.T) {
	anon := mustLoadSDKParityFixture(t, "anonymous.json")
	anonResponse, err := json.Marshal(anon.Response)
	if err != nil {
		t.Fatalf("re-encode anonymous response: %v", err)
	}
	var anonAuth AuthResponse
	if err := json.Unmarshal(anonResponse, &anonAuth); err != nil {
		t.Fatalf("decode anonymous AuthResponse: %v", err)
	}
	if anonAuth.User.IsAnonymous == nil || !*anonAuth.User.IsAnonymous {
		t.Fatalf("expected IsAnonymous=true, got %+v", anonAuth.User.IsAnonymous)
	}
	if anonAuth.User.Email != "" {
		t.Fatalf("expected empty email for anonymous user, got %q", anonAuth.User.Email)
	}

	linked := mustLoadSDKParityFixture(t, "link_email.json")
	linkedResponse, err := json.Marshal(linked.Response)
	if err != nil {
		t.Fatalf("re-encode link_email response: %v", err)
	}
	var linkedAuth AuthResponse
	if err := json.Unmarshal(linkedResponse, &linkedAuth); err != nil {
		t.Fatalf("decode link_email AuthResponse: %v", err)
	}
	if linkedAuth.User.Email != "upgraded@example.com" {
		t.Fatalf("unexpected linked email %q", linkedAuth.User.Email)
	}
	if linkedAuth.User.LinkedAt == nil || *linkedAuth.User.LinkedAt == "" {
		t.Fatalf("expected linked_at to be populated, got %+v", linkedAuth.User.LinkedAt)
	}
	if linkedAuth.User.IsAnonymous != nil && *linkedAuth.User.IsAnonymous {
		t.Fatalf("expected non-anonymous linked user, got %+v", linkedAuth.User.IsAnonymous)
	}
}

func TestContractAuthResponseJSONShape(t *testing.T) {
	raw := []byte(`{"token":"jwt_stage3","refreshToken":"refresh_stage3","user":{"id":"usr_1","email":"dev@allyourbase.io","email_verified":true,"created_at":"2026-01-01T00:00:00Z","updated_at":null}}`)
	var out AuthResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Token != "jwt_stage3" || out.RefreshToken != "refresh_stage3" || out.User.ID != "usr_1" {
		t.Fatalf("bad parse: %+v", out)
	}
	if out.User.Email != "dev@allyourbase.io" {
		t.Fatalf("bad email parse: %+v", out.User)
	}
	if out.User.EmailVerified == nil || *out.User.EmailVerified != true {
		t.Fatalf("bad email verified parse: %+v", out.User)
	}
	if out.User.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("bad createdAt parse: %+v", out.User)
	}
	if out.User.UpdatedAt != nil {
		t.Fatalf("expected nil updatedAt, got: %+v", out.User.UpdatedAt)
	}
}

func TestContractListResponseJSONShape(t *testing.T) {
	raw := []byte(`{"items":[{"id":"rec_1","title":"First","_highlight":"<mark>First</mark>"},{"id":"rec_2","title":"Second"}],"page":1,"perPage":2,"totalItems":2,"totalPages":1,"facets":{"category":[{"value":"dessert","count":2}]}}`)
	var out ListResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.PerPage != 2 || len(out.Items) != 2 {
		t.Fatalf("bad parse: %+v", out)
	}
	if out.Page != 1 || out.TotalItems != 2 || out.TotalPages != 1 {
		t.Fatalf("bad metadata parse: %+v", out)
	}
	if out.Items[0]["title"] != "First" || out.Items[1]["title"] != "Second" {
		t.Fatalf("bad item order parse: %+v", out.Items)
	}
	if out.Items[0]["_highlight"] != "<mark>First</mark>" {
		t.Fatalf("bad highlight parse: %+v", out.Items[0])
	}
	if out.Facets["category"][0].Value != "dessert" || out.Facets["category"][0].Count != 2 {
		t.Fatalf("bad facets parse: %+v", out.Facets)
	}
}

func TestContractCursorListResponseJSONShape(t *testing.T) {
	raw := []byte(`{"items":[{"id":"rec_1","title":"First"}],"perPage":2,"nextCursor":"cursor_2","facets":{"priority":[{"value":1,"count":2}]}}`)
	var out ListResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.NextCursor == nil || *out.NextCursor != "cursor_2" {
		t.Fatalf("bad nextCursor parse: %+v", out.NextCursor)
	}
	if out.Page != 0 || out.TotalItems != 0 || out.TotalPages != 0 {
		t.Fatalf("expected offset fields to stay zero-valued for cursor envelope, got %+v", out)
	}
	if out.Facets["priority"][0].Value != float64(1) || out.Facets["priority"][0].Count != 2 {
		t.Fatalf("bad numeric facet parse: %+v", out.Facets)
	}
}

func TestContractStorageObjectJSONShape(t *testing.T) {
	raw := []byte(`{"id":"file_abc123","bucket":"uploads","name":"document.pdf","size":1024,"contentType":"application/pdf","userId":"usr_1","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T12:30:00Z"}`)
	var out StorageObject
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.ContentType != "application/pdf" || out.Name != "document.pdf" || out.Bucket != "uploads" {
		t.Fatalf("bad parse: %+v", out)
	}
	if out.UserID == nil || *out.UserID != "usr_1" {
		t.Fatalf("bad userId parse: %+v", out)
	}
	if out.UpdatedAt == nil || *out.UpdatedAt != "2026-01-02T12:30:00Z" {
		t.Fatalf("bad updatedAt parse: %+v", out)
	}
}

func TestContractStorageListResponseJSONShape(t *testing.T) {
	raw := []byte(`{"items":[{"id":"file_1","bucket":"uploads","name":"doc1.pdf","size":1024,"contentType":"application/pdf","userId":"usr_1","createdAt":"2026-01-01T00:00:00Z","updatedAt":null},{"id":"file_2","bucket":"uploads","name":"image.png","size":2048,"contentType":"image/png","userId":null,"createdAt":"2026-01-02T00:00:00Z","updatedAt":null}],"totalItems":2}`)
	var out StorageListResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.TotalItems != 2 || len(out.Items) != 2 {
		t.Fatalf("bad parse: %+v", out)
	}
	if out.Items[0].UserID == nil || *out.Items[0].UserID != "usr_1" {
		t.Fatalf("bad first userId parse: %+v", out.Items[0])
	}
	if out.Items[1].UserID != nil {
		t.Fatalf("expected nil second userId, got: %+v", out.Items[1].UserID)
	}
	if out.Items[0].UpdatedAt != nil || out.Items[1].UpdatedAt != nil {
		t.Fatalf("expected nil updatedAt for list fixtures: %+v", out.Items)
	}
}

func TestContractErrorResponseNumericCodeShape(t *testing.T) {
	raw := []byte(`{"code":403,"message":"forbidden","data":{"resource":"posts"},"doc_url":"https://allyourbase.io/docs/errors#forbidden"}`)
	err := normalizeError(403, "Forbidden", raw)
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Code != "403" || apiErr.Message != "forbidden" {
		t.Fatalf("bad parse: %+v", apiErr)
	}
	if apiErr.Data["resource"] != "posts" || apiErr.DocURL != "https://allyourbase.io/docs/errors#forbidden" {
		t.Fatalf("bad details parse: %+v", apiErr)
	}
}

func TestContractErrorResponseNumericCodePreservesNonIntegerValue(t *testing.T) {
	raw := []byte(`{"code":403.5,"message":"forbidden"}`)
	err := normalizeError(403, "Forbidden", raw)
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Code != "403.5" {
		t.Fatalf("expected fractional code to be preserved, got: %q", apiErr.Code)
	}
}

func TestContractErrorResponseStringCodeShape(t *testing.T) {
	raw := []byte(`{"code":"auth/missing-refresh-token","message":"Missing refresh token","data":{"detail":"refresh token not available"}}`)
	err := normalizeError(400, "Bad Request", raw)
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Code != "auth/missing-refresh-token" || apiErr.Message != "Missing refresh token" {
		t.Fatalf("bad parse: %+v", apiErr)
	}
	if apiErr.Data["detail"] != "refresh token not available" {
		t.Fatalf("bad details parse: %+v", apiErr)
	}
}
