package fbmigrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

// migrateAuthUsers inserts Firebase users from the export into _ayb_users, validating the table exists and encoding password hashes. It skips disabled, anonymous, and phone-only users and tracks migration statistics for each row inserted.
func (m *Migrator) migrateAuthUsers(ctx context.Context, tx *sql.Tx, users []FirebaseUser, hashConfig *FirebaseHashConfig, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Auth users", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, len(users))
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating auth users...")

	// Ensure _ayb_users exists.
	var tableExists bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = '_ayb_users'
		)
	`).Scan(&tableExists)
	if err != nil || !tableExists {
		return fmt.Errorf("_ayb_users table not found — run 'ayb start' or 'ayb migrate up' first")
	}

	for i, u := range users {
		// Skip disabled, anonymous, and phone-only users.
		if u.Disabled {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped user %s (disabled)\n", u.LocalID)
			}
			m.progress.Progress(phase, i+1, len(users))
			continue
		}
		if IsAnonymousUser(u) || IsPhoneOnlyUser(u) {
			m.stats.Skipped++
			if m.verbose {
				fmt.Fprintf(m.output, "  skipped user %s (anonymous/phone-only)\n", u.LocalID)
			}
			m.progress.Progress(phase, i+1, len(users))
			continue
		}

		if !IsEmailUser(u) {
			m.stats.Skipped++
			m.progress.Progress(phase, i+1, len(users))
			continue
		}

		passwordHash := "$none$"
		if IsPasswordUser(u) {
			passwordHash = EncodeFirebaseScryptHash(u.PasswordHash, u.Salt, hashConfig)
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s (%s) verified=%v\n", u.Email, u.LocalID, u.EmailVerified)
		}

		aybUserID := FirebaseIDToUUID(u.LocalID)
		result, err := tx.ExecContext(ctx,
			`INSERT INTO _ayb_users (id, email, password_hash, email_verified, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (id) DO NOTHING`,
			aybUserID, strings.ToLower(u.Email), passwordHash,
			u.EmailVerified, parseEpochMs(u.CreatedAt), parseEpochMs(u.CreatedAt),
		)
		if err != nil {
			m.stats.Errors = append(m.stats.Errors, fmt.Sprintf("inserting user %s: %v", u.Email, err))
			m.progress.Progress(phase, i+1, len(users))
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			m.stats.Users++
		}
		m.progress.Progress(phase, i+1, len(users))
	}

	m.progress.CompletePhase(phase, m.stats.Users, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d users migrated (%d skipped)\n", m.stats.Users, m.stats.Skipped)
	return nil
}

// migrateOAuthLinks inserts Firebase OAuth provider identities into _ayb_oauth_accounts for email-enabled users, normalizing provider names and using the user email as fallback if the provider email is missing.
func (m *Migrator) migrateOAuthLinks(ctx context.Context, tx *sql.Tx, users []FirebaseUser, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "OAuth", Index: phaseIdx, Total: totalPhases}
	m.progress.StartPhase(phase, 0)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating OAuth identities...")

	for _, u := range users {
		if u.Disabled || !IsEmailUser(u) {
			continue
		}
		for _, p := range OAuthProviders(u) {
			providerName := NormalizeProvider(p.ProviderID)
			email := p.Email
			if email == "" {
				email = u.Email
			}

			if m.verbose {
				fmt.Fprintf(m.output, "  %s -> %s (%s)\n", providerName, email, p.RawID)
			}

			aybUserID := FirebaseIDToUUID(u.LocalID)
			result, err := tx.ExecContext(ctx,
				`INSERT INTO _ayb_oauth_accounts (user_id, provider, provider_user_id, email, name, created_at)
				 VALUES ($1, $2, $3, $4, $5, $6)
				 ON CONFLICT (provider, provider_user_id) DO NOTHING`,
				aybUserID, providerName, p.RawID, email, p.DisplayName, parseEpochMs(u.CreatedAt),
			)
			if err != nil {
				m.stats.Errors = append(m.stats.Errors,
					fmt.Sprintf("inserting OAuth for user %s: %v", u.LocalID, err))
				continue
			}
			if n, _ := result.RowsAffected(); n > 0 {
				m.stats.OAuthLinks++
			}
		}
	}

	m.progress.CompletePhase(phase, m.stats.OAuthLinks, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d OAuth identities migrated\n", m.stats.OAuthLinks)
	return nil
}

// migrateFirestoreData creates database tables for each Firestore collection with a GIN index, flattens document fields to JSON, and inserts documents as rows. It reports progress per document and tracks migration statistics.
func (m *Migrator) migrateFirestoreData(ctx context.Context, tx *sql.Tx, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Firestore", Index: phaseIdx, Total: totalPhases}

	collections, err := ParseFirestoreExport(m.opts.FirestoreExportPath)
	if err != nil {
		return err
	}

	var totalDocs int
	for _, c := range collections {
		totalDocs += len(c.Documents)
	}

	m.progress.StartPhase(phase, totalDocs)
	start := time.Now()

	fmt.Fprintln(m.output, "Migrating Firestore data...")

	processed := 0
	for _, coll := range collections {
		tableName := NormalizeCollectionName(coll.Name)

		ddl := CreateCollectionTableSQL(tableName)
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("creating table %s: %w", tableName, err)
		}

		indexSQL := CreateCollectionIndexSQL(tableName)
		if _, err := tx.ExecContext(ctx, indexSQL); err != nil {
			m.progress.Warn(fmt.Sprintf("creating index on %s: %v", tableName, err))
		}

		m.stats.Collections++

		for _, doc := range coll.Documents {
			flatFields := FlattenFirestoreFields(doc.Fields)
			jsonData, err := json.Marshal(flatFields)
			if err != nil {
				m.stats.Errors = append(m.stats.Errors,
					fmt.Sprintf("marshaling document %s in %s: %v", doc.ID, tableName, err))
				processed++
				m.progress.Progress(phase, processed, totalDocs)
				continue
			}

			result, err := tx.ExecContext(ctx,
				fmt.Sprintf(`INSERT INTO %q ("id", "data") VALUES ($1, $2) ON CONFLICT ("id") DO NOTHING`, tableName),
				doc.ID, jsonData,
			)
			if err != nil {
				m.stats.Errors = append(m.stats.Errors,
					fmt.Sprintf("inserting document %s into %s: %v", doc.ID, tableName, err))
				processed++
				m.progress.Progress(phase, processed, totalDocs)
				continue
			}
			if n, _ := result.RowsAffected(); n > 0 {
				m.stats.Documents++
			}
			processed++
			m.progress.Progress(phase, processed, totalDocs)
		}

		if m.verbose {
			fmt.Fprintf(m.output, "  %s: %d documents\n", tableName, len(coll.Documents))
		}
	}

	m.progress.CompletePhase(phase, totalDocs, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d documents across %d collections\n", m.stats.Documents, m.stats.Collections)
	return nil
}

// parseEpochMs converts a millisecond epoch string to time.Time.
func parseEpochMs(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	var ms int64
	if _, err := fmt.Sscanf(s, "%d", &ms); err != nil {
		return time.Now()
	}
	return time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond))
}
