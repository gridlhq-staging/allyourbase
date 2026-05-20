package emailtemplates

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store handles database CRUD for custom email templates.
type Store struct {
	pool *pgxpool.Pool
}

// TemplateStore defines storage operations needed by Service.
type TemplateStore interface {
	Upsert(ctx context.Context, key, subjectTpl, htmlTpl string) (*Template, error)
	Get(ctx context.Context, key string) (*Template, error)
	List(ctx context.Context) ([]*Template, error)
	Delete(ctx context.Context, key string) error
	SetEnabled(ctx context.Context, key string, enabled bool) error
}

// NewStore creates a new template store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Upsert creates or updates a custom template override. It validates the key
// format and parses both templates to catch syntax errors before saving.
func (s *Store) Upsert(ctx context.Context, key, subjectTpl, htmlTpl string) (*Template, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	if len(subjectTpl) > MaxSubjectLen {
		return nil, fmt.Errorf("%w: subject exceeds %d characters", ErrTooLarge, MaxSubjectLen)
	}
	if len(htmlTpl) > MaxHTMLLen {
		return nil, fmt.Errorf("%w: html exceeds %d characters", ErrTooLarge, MaxHTMLLen)
	}
	if _, err := parseSubject(key, subjectTpl); err != nil {
		return nil, fmt.Errorf("%w: subject: %v", ErrParseFailed, err)
	}
	if _, err := parseHTML(key, htmlTpl); err != nil {
		return nil, fmt.Errorf("%w: html: %v", ErrParseFailed, err)
	}

	var t Template
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_email_templates (template_key, subject_template, html_template)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (template_key) DO UPDATE
		   SET subject_template = EXCLUDED.subject_template,
		       html_template = EXCLUDED.html_template,
		       updated_at = now()
		 RETURNING id, template_key, subject_template, html_template, enabled, created_at, updated_at`,
		key, subjectTpl, htmlTpl,
	).Scan(&t.ID, &t.TemplateKey, &t.SubjectTemplate, &t.HTMLTemplate,
		&t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting template %q: %w", key, err)
	}
	return &t, nil
}

// Get returns a custom template by key, or ErrNotFound.
func (s *Store) Get(ctx context.Context, key string) (*Template, error) {
	var t Template
	err := s.pool.QueryRow(ctx,
		`SELECT id, template_key, subject_template, html_template, enabled, created_at, updated_at
		 FROM _ayb_email_templates WHERE template_key = $1`, key,
	).Scan(&t.ID, &t.TemplateKey, &t.SubjectTemplate, &t.HTMLTemplate,
		&t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting template %q: %w", key, err)
	}
	return &t, nil
}

// List returns all custom template overrides.
func (s *Store) List(ctx context.Context) ([]*Template, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, template_key, subject_template, html_template, enabled, created_at, updated_at
		 FROM _ayb_email_templates ORDER BY template_key`)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	defer rows.Close()

	var templates []*Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.TemplateKey, &t.SubjectTemplate, &t.HTMLTemplate,
			&t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning template row: %w", err)
		}
		templates = append(templates, &t)
	}
	return templates, rows.Err()
}

// Delete removes a custom template override by key.
func (s *Store) Delete(ctx context.Context, key string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_email_templates WHERE template_key = $1`, key)
	if err != nil {
		return fmt.Errorf("deleting template %q: %w", key, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetEnabled toggles the enabled flag on a custom template.
func (s *Store) SetEnabled(ctx context.Context, key string, enabled bool) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE _ayb_email_templates SET enabled = $2, updated_at = now()
		 WHERE template_key = $1`, key, enabled)
	if err != nil {
		return fmt.Errorf("toggling template %q enabled: %w", key, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
