// Package auth Manages default tenant lookup and automatic creation for authenticated users, ensuring every user has at least one tenant available for sessions.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/jackc/pgx/v5"
)

type authWriteScope struct {
	database authDatabase
	tenants  tenant.TransactionalWriter
}

func (s *Service) poolWriteScope() authWriteScope {
	return authWriteScope{
		database: s.pool,
		tenants:  tenant.NewService(s.pool, s.logger),
	}
}

func (s *Service) transactionWriteScope(tx pgx.Tx) authWriteScope {
	return authWriteScope{
		database: tx,
		tenants:  tenant.NewTransactionalWriter(tx, s.logger),
	}
}

func (s *Service) issueTokensInScope(ctx context.Context, user *User, firstFactorMethod string, scope authWriteScope) (*User, string, string, error) {
	sessionOpts := firstFactorSessionOptions(firstFactorMethod)
	sessionID, refreshToken, err := s.createSessionInDatabase(ctx, scope.database, user.ID, sessionOpts)
	if err != nil {
		return nil, "", "", fmt.Errorf("creating session: %w", err)
	}
	if sessionOpts == nil {
		sessionOpts = &tokenOptions{}
	}
	sessionOpts.SessionID = sessionID

	opts, err := s.sessionTokenOptionsInScope(ctx, user, sessionOpts, scope)
	if err != nil {
		return nil, "", "", fmt.Errorf("resolving session tenant: %w", err)
	}
	token, err := s.generateTokenWithOpts(ctx, user, opts)
	if err != nil {
		return nil, "", "", fmt.Errorf("generating token: %w", err)
	}
	return user, token, refreshToken, nil
}

func cloneTokenOptions(opts *tokenOptions) *tokenOptions {
	if opts == nil {
		return &tokenOptions{}
	}
	cloned := *opts
	if len(opts.AMR) > 0 {
		cloned.AMR = append([]string(nil), opts.AMR...)
	}
	return &cloned
}

func (s *Service) sessionTokenOptions(ctx context.Context, user *User, opts *tokenOptions) (*tokenOptions, error) {
	return s.sessionTokenOptionsInScope(ctx, user, opts, s.poolWriteScope())
}

func (s *Service) sessionTokenOptionsInScope(ctx context.Context, user *User, opts *tokenOptions, scope authWriteScope) (*tokenOptions, error) {
	resolved := cloneTokenOptions(opts)
	if user == nil || user.IsAnonymous || strings.TrimSpace(user.ID) == "" {
		return resolved, nil
	}

	tenantID, err := s.ensureDefaultTenantIDInScope(ctx, user, scope)
	if err != nil {
		return nil, err
	}
	resolved.TenantID = tenantID
	return resolved, nil
}

// ensureDefaultTenantID returns the user's default tenant ID, automatically creating a personal tenant if one does not exist and adding the user as its owner.
func (s *Service) ensureDefaultTenantID(ctx context.Context, user *User) (string, error) {
	return s.ensureDefaultTenantIDInScope(ctx, user, s.poolWriteScope())
}

func (s *Service) ensureDefaultTenantIDInScope(ctx context.Context, user *User, scope authWriteScope) (string, error) {
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return "", nil
	}

	tenantID, err := s.lookupDefaultTenantIDInDatabase(ctx, scope.database, user.ID)
	if err != nil {
		return "", err
	}
	if tenantID != "" {
		return tenantID, nil
	}

	personalTenant, err := scope.tenants.CreateTenant(
		ctx,
		personalTenantName(user),
		"user-"+strings.ToLower(user.ID),
		"schema",
		"free",
		"default",
		nil,
		"auth-personal-tenant:"+user.ID,
	)
	if err != nil {
		return "", fmt.Errorf("creating default tenant for user %s: %w", user.ID, err)
	}
	if _, err := scope.tenants.AddMembership(ctx, personalTenant.ID, user.ID, tenant.MemberRoleOwner); err != nil && !errors.Is(err, tenant.ErrMembershipExists) {
		return "", fmt.Errorf("adding default tenant membership for user %s: %w", user.ID, err)
	}
	return personalTenant.ID, nil
}

// lookupDefaultTenantID queries the database to find the user's highest-priority tenant membership, ordered by role precedence (owner, admin, member) and creation time, returning the tenant ID or an empty string if no membership exists.
func (s *Service) lookupDefaultTenantID(ctx context.Context, userID string) (string, error) {
	return s.lookupDefaultTenantIDInDatabase(ctx, s.pool, userID)
}

func (s *Service) lookupDefaultTenantIDInDatabase(ctx context.Context, database authDatabase, userID string) (string, error) {
	var tenantID string
	err := database.QueryRow(ctx, `
		SELECT m.tenant_id
		  FROM _ayb_tenant_memberships m
		  JOIN _ayb_tenants t ON t.id = m.tenant_id
		 WHERE m.user_id = $1
		   AND t.state <> 'deleted'
		 ORDER BY CASE m.role
		     WHEN 'owner' THEN 0
		     WHEN 'admin' THEN 1
		     WHEN 'member' THEN 2
		     ELSE 3
		 END,
		 m.created_at ASC
		 LIMIT 1
	`, userID).Scan(&tenantID)
	if err == nil {
		return tenantID, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return "", fmt.Errorf("resolving default tenant for user %s: %w", userID, err)
}

func personalTenantName(user *User) string {
	if user == nil {
		return "Personal Workspace"
	}
	if email := strings.TrimSpace(user.Email); email != "" {
		return email
	}
	if phone := strings.TrimSpace(user.Phone); phone != "" {
		return phone
	}
	return "Personal Workspace"
}
