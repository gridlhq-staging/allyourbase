package api

import (
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

func TestRPCNotifyRejectsUnknownTargetBeforeExecution(t *testing.T) {
	t.Parallel()

	h := newRPCRequestPathHarness()
	w := h.doRPC("get_profile", "", map[string]string{
		"X-Notify-Table":  "missing_table",
		"X-Notify-Action": "update",
	})

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, decodeError(t, w).Message, "collection not found")
	h.assertNoExecution(t)
	h.assertNoPublish(t)
}

func TestRPCNotifyEnforcesTargetTableScopeBeforeExecution(t *testing.T) {
	t.Parallel()

	h := newRPCRequestPathHarness()
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user_1"},
		AllowedTables:    []string{"logs"},
	}
	w := h.doRPCWithClaims("get_profile", "", map[string]string{
		"X-Notify-Table":  "users",
		"X-Notify-Action": "update",
	}, claims)

	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, decodeError(t, w).Message, "does not have access")
	h.assertNoExecution(t)
	h.assertNoPublish(t)
}
