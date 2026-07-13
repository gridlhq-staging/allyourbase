package allyourbase

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestContractSearchSynonymsFixturesDecodeThroughModelOwners(t *testing.T) {
	resData := mustLoadContractFixture(t, "search_synonyms_response.json")
	var res SearchSynonymsResponse
	if err := json.Unmarshal(resData, &res); err != nil {
		t.Fatalf("decode search_synonyms_response: %v", err)
	}
	expectedResponseTerms := [][]string{
		{"new york", "nyc"},
		{"science fiction", "scifi"},
	}
	if !reflect.DeepEqual(searchSynonymsTerms(res.Groups), expectedResponseTerms) {
		t.Fatalf("unexpected response groups: %+v", res.Groups)
	}

	reqData := mustLoadContractFixture(t, "search_synonyms_request.json")
	var req SearchSynonymsRequest
	if err := json.Unmarshal(reqData, &req); err != nil {
		t.Fatalf("decode search_synonyms_request: %v", err)
	}
	actualData, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal search_synonyms_request: %v", err)
	}

	var expectedEnvelope map[string]any
	if err := json.Unmarshal(reqData, &expectedEnvelope); err != nil {
		t.Fatalf("decode expected search_synonyms_request envelope: %v", err)
	}
	var actualEnvelope map[string]any
	if err := json.Unmarshal(actualData, &actualEnvelope); err != nil {
		t.Fatalf("decode actual search_synonyms_request envelope: %v", err)
	}
	if !reflect.DeepEqual(actualEnvelope, expectedEnvelope) {
		t.Fatalf("request envelope mismatch\nactual:   %#v\nexpected: %#v", actualEnvelope, expectedEnvelope)
	}
	expectedRequestTerms := [][]string{
		{"scifi", "science fiction"},
		{"nyc", "new york"},
	}
	if !reflect.DeepEqual(searchSynonymsTerms(req.Groups), expectedRequestTerms) {
		t.Fatalf("unexpected request groups: %+v", req.Groups)
	}
}

func searchSynonymsTerms(groups []SearchSynonymsGroup) [][]string {
	terms := make([][]string, len(groups))
	for i, group := range groups {
		terms[i] = group.Terms
	}
	return terms
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

func TestContractWebAuthnLoginBeginResponseFixture(t *testing.T) {
	data := mustLoadContractFixture(t, "webauthn_login_begin_response.json")
	var out WebAuthnLoginBeginResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode webauthn_login_begin_response: %v", err)
	}
	if out.ChallengeID != "webauthn_challenge_fixture" {
		t.Fatalf("unexpected challenge_id %q", out.ChallengeID)
	}
	if out.Options.Challenge != "webauthn_login_begin_challenge" {
		t.Fatalf("unexpected challenge %q", out.Options.Challenge)
	}
	if out.Options.RPID != "127.0.0.1" {
		t.Fatalf("unexpected rpId %q", out.Options.RPID)
	}
	if len(out.Options.AllowCredentials) == 0 {
		t.Fatalf("expected allowCredentials to be populated")
	}
	first := out.Options.AllowCredentials[0]
	if first.ID != "webauthn_login_begin_credential_a" || first.Type != "public-key" {
		t.Fatalf("unexpected first allow credential %+v", first)
	}

	field, ok := reflect.TypeOf(WebAuthnLoginBeginResponse{}).FieldByName("ChallengeID")
	if !ok {
		t.Fatalf("ChallengeID field missing")
	}
	if field.Tag.Get("json") != "challenge_id" {
		t.Fatalf("ChallengeID json tag must own challenge_id, got %q", field.Tag.Get("json"))
	}
}

func TestContractWebAuthnDiscoverableLoginFixturesReuseCanonicalModels(t *testing.T) {
	beginData := mustLoadContractFixture(t, "webauthn_discover_begin_response.json")
	var begin WebAuthnLoginBeginResponse
	if err := json.Unmarshal(beginData, &begin); err != nil {
		t.Fatalf("decode webauthn_discover_begin_response: %v", err)
	}
	if begin.ChallengeID != "webauthn_discover_challenge_fixture" {
		t.Fatalf("unexpected challenge_id %q", begin.ChallengeID)
	}
	if begin.Options.Challenge != "webauthn_discover_challenge" {
		t.Fatalf("unexpected challenge %q", begin.Options.Challenge)
	}
	if begin.Options.RPID != "127.0.0.1" {
		t.Fatalf("unexpected rpId %q", begin.Options.RPID)
	}
	if begin.Options.Timeout != 300000 {
		t.Fatalf("unexpected timeout %d", begin.Options.Timeout)
	}
	if len(begin.Options.AllowCredentials) != 0 {
		t.Fatalf("expected absent or empty allowCredentials, got %+v", begin.Options.AllowCredentials)
	}

	finishData := mustLoadContractFixture(t, "webauthn_discover_finish_request.json")
	var finish WebAuthnLoginFinishRequest
	if err := json.Unmarshal(finishData, &finish); err != nil {
		t.Fatalf("decode webauthn_discover_finish_request: %v", err)
	}
	if finish.ChallengeID != "webauthn_discover_challenge_fixture" {
		t.Fatalf("unexpected challenge_id %q", finish.ChallengeID)
	}
	var assertion map[string]any
	if err := json.Unmarshal(finish.AssertionResponse, &assertion); err != nil {
		t.Fatalf("decode assertion_response: %v", err)
	}
	if assertion["id"] != "webauthn_discover_credential" {
		t.Fatalf("unexpected assertion_response.id %#v", assertion["id"])
	}
	actualData, err := json.Marshal(finish)
	if err != nil {
		t.Fatalf("marshal webauthn_discover_finish_request: %v", err)
	}
	var expectedEnvelope map[string]any
	if err := json.Unmarshal(finishData, &expectedEnvelope); err != nil {
		t.Fatalf("decode expected webauthn_discover_finish_request envelope: %v", err)
	}
	var actualEnvelope map[string]any
	if err := json.Unmarshal(actualData, &actualEnvelope); err != nil {
		t.Fatalf("decode actual webauthn_discover_finish_request envelope: %v", err)
	}
	if !reflect.DeepEqual(actualEnvelope, expectedEnvelope) {
		t.Fatalf("request envelope mismatch\nactual:   %#v\nexpected: %#v", actualEnvelope, expectedEnvelope)
	}

	authData := mustLoadContractFixture(t, "auth_response.json")
	var auth AuthResponse
	if err := json.Unmarshal(authData, &auth); err != nil {
		t.Fatalf("decode auth_response: %v", err)
	}
	if auth.Token != "jwt_stage3" {
		t.Fatalf("unexpected token %q", auth.Token)
	}
	if auth.RefreshToken != "refresh_stage3" {
		t.Fatalf("unexpected refresh token %q", auth.RefreshToken)
	}
	if auth.User.Email != "dev@allyourbase.io" {
		t.Fatalf("unexpected email %q", auth.User.Email)
	}
}

func TestContractWebAuthnFinishReusesAuthResponseFixture(t *testing.T) {
	data := mustLoadContractFixture(t, "auth_response.json")
	var out AuthResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode auth_response: %v", err)
	}
	if out.Token != "jwt_stage3" {
		t.Fatalf("unexpected token %q", out.Token)
	}
	if out.RefreshToken != "refresh_stage3" {
		t.Fatalf("unexpected refresh token %q", out.RefreshToken)
	}
	if out.User.ID != "usr_1" {
		t.Fatalf("unexpected user id %q", out.User.ID)
	}
	if out.User.Email != "dev@allyourbase.io" {
		t.Fatalf("unexpected email %q", out.User.Email)
	}
	if out.User.EmailVerified == nil || !*out.User.EmailVerified {
		t.Fatalf("expected email_verified=true, got %+v", out.User.EmailVerified)
	}
	if out.User.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("unexpected created_at %q", out.User.CreatedAt)
	}
	if out.User.UpdatedAt != nil {
		t.Fatalf("expected updated_at nil, got %+v", out.User.UpdatedAt)
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

func TestContractWebAuthnEnrollBeginResponseFixture(t *testing.T) {
	data := mustLoadContractFixture(t, "webauthn_enroll_begin_response.json")
	var out WebAuthnEnrollBeginResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode webauthn_enroll_begin_response: %v", err)
	}
	if out.Challenge != "webauthn_enroll_begin_challenge" {
		t.Fatalf("unexpected challenge %q", out.Challenge)
	}
	if out.RP.ID != "127.0.0.1" || out.RP.Name != "Allyourbase" {
		t.Fatalf("unexpected rp %+v", out.RP)
	}
	if out.Timeout != 300000 {
		t.Fatalf("unexpected timeout %d", out.Timeout)
	}
	if out.User.ID != "webauthn_enroll_user_id" {
		t.Fatalf("unexpected user id %q", out.User.ID)
	}
	if out.User.Name != "webauthn-e2e@example.com" {
		t.Fatalf("unexpected user name %q", out.User.Name)
	}
	if out.User.DisplayName != "webauthn-e2e@example.com" {
		t.Fatalf("unexpected user displayName %q", out.User.DisplayName)
	}
	if out.AuthenticatorSelection.ResidentKey != "preferred" {
		t.Fatalf("unexpected residentKey %q", out.AuthenticatorSelection.ResidentKey)
	}
	wantAlgs := []int{-7, -35, -36, -257, -258, -259, -37, -38, -39, -8}
	if len(out.PubKeyCredParams) != len(wantAlgs) {
		t.Fatalf("expected %d pubKeyCredParams, got %d", len(wantAlgs), len(out.PubKeyCredParams))
	}
	for i, want := range wantAlgs {
		if out.PubKeyCredParams[i].Alg != want {
			t.Fatalf("pubKeyCredParams[%d].alg = %d, want %d", i, out.PubKeyCredParams[i].Alg, want)
		}
	}
}

func TestContractWebAuthnEnrollConfirmRequestFixture(t *testing.T) {
	data := mustLoadContractFixture(t, "webauthn_enroll_confirm_request.json")
	var req WebAuthnEnrollConfirmRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("decode webauthn_enroll_confirm_request: %v", err)
	}
	if req.DisplayName != "Primary security key" {
		t.Fatalf("unexpected display_name %q", req.DisplayName)
	}
	actualData, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal webauthn_enroll_confirm_request: %v", err)
	}
	var expectedEnvelope map[string]any
	if err := json.Unmarshal(data, &expectedEnvelope); err != nil {
		t.Fatalf("decode expected webauthn_enroll_confirm_request envelope: %v", err)
	}
	var actualEnvelope map[string]any
	if err := json.Unmarshal(actualData, &actualEnvelope); err != nil {
		t.Fatalf("decode actual webauthn_enroll_confirm_request envelope: %v", err)
	}
	if !reflect.DeepEqual(actualEnvelope, expectedEnvelope) {
		t.Fatalf("request envelope mismatch\nactual:   %#v\nexpected: %#v", actualEnvelope, expectedEnvelope)
	}
}

func TestContractWebAuthnEnrollConfirmResponseFixture(t *testing.T) {
	data := mustLoadContractFixture(t, "webauthn_enroll_confirm_response.json")
	var out WebAuthnEnrollConfirmResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode webauthn_enroll_confirm_response: %v", err)
	}
	if out.Message != "WebAuthn MFA enrollment confirmed" {
		t.Fatalf("unexpected message %q", out.Message)
	}
}

func TestContractWebAuthnMFAChallengeResponseFixture(t *testing.T) {
	data := mustLoadContractFixture(t, "webauthn_mfa_challenge_response.json")
	var out WebAuthnMFAChallengeResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode webauthn_mfa_challenge_response: %v", err)
	}
	if out.ChallengeID != "webauthn_mfa_challenge_fixture" {
		t.Fatalf("unexpected challenge_id %q", out.ChallengeID)
	}
	if out.Options.Challenge != "webauthn_mfa_challenge" {
		t.Fatalf("unexpected challenge %q", out.Options.Challenge)
	}
	if out.Options.RPID != "127.0.0.1" {
		t.Fatalf("unexpected rpId %q", out.Options.RPID)
	}
	if out.Options.Timeout != 300000 {
		t.Fatalf("unexpected timeout %d", out.Options.Timeout)
	}
	if len(out.Options.AllowCredentials) == 0 {
		t.Fatalf("expected allowCredentials to be populated")
	}
	first := out.Options.AllowCredentials[0]
	if first.ID != "webauthn_mfa_credential_a" || first.Type != "public-key" {
		t.Fatalf("unexpected first allow credential %+v", first)
	}

	field, ok := reflect.TypeOf(WebAuthnMFAChallengeResponse{}).FieldByName("ChallengeID")
	if !ok {
		t.Fatalf("ChallengeID field missing")
	}
	if field.Tag.Get("json") != "challenge_id" {
		t.Fatalf("ChallengeID json tag must own challenge_id, got %q", field.Tag.Get("json"))
	}
}

func TestContractWebAuthnMFAVerifyRequestFixture(t *testing.T) {
	data := mustLoadContractFixture(t, "webauthn_mfa_verify_request.json")
	var req WebAuthnMFAVerifyRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("decode webauthn_mfa_verify_request: %v", err)
	}
	if req.ChallengeID != "webauthn_mfa_challenge_fixture" {
		t.Fatalf("unexpected challenge_id %q", req.ChallengeID)
	}
	actualData, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal webauthn_mfa_verify_request: %v", err)
	}
	var expectedEnvelope map[string]any
	if err := json.Unmarshal(data, &expectedEnvelope); err != nil {
		t.Fatalf("decode expected webauthn_mfa_verify_request envelope: %v", err)
	}
	var actualEnvelope map[string]any
	if err := json.Unmarshal(actualData, &actualEnvelope); err != nil {
		t.Fatalf("decode actual webauthn_mfa_verify_request envelope: %v", err)
	}
	if !reflect.DeepEqual(actualEnvelope, expectedEnvelope) {
		t.Fatalf("request envelope mismatch\nactual:   %#v\nexpected: %#v", actualEnvelope, expectedEnvelope)
	}
}

func TestContractWebAuthnMFAVerifyResponseFixture(t *testing.T) {
	data := mustLoadContractFixture(t, "webauthn_mfa_verify_response.json")
	var out AuthResponse
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode webauthn_mfa_verify_response: %v", err)
	}
	if out.Token != "jwt_webauthn_mfa" {
		t.Fatalf("unexpected token %q", out.Token)
	}
	if out.RefreshToken != "refresh_webauthn_mfa" {
		t.Fatalf("unexpected refresh token %q", out.RefreshToken)
	}
	if out.User.Email != "webauthn-e2e@example.com" {
		t.Fatalf("unexpected email %q", out.User.Email)
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
