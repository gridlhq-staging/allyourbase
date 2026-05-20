package emailtemplates

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/mailer"
)

// Sentinel errors.
var (
	ErrNotFound     = errors.New("template not found")
	ErrNoTemplate   = errors.New("no template exists for key")
	ErrInvalidKey   = errors.New("invalid template key format")
	ErrParseFailed  = errors.New("template parse error")
	ErrRenderFailed = errors.New("template render error")
	ErrTooLarge     = errors.New("template exceeds size limit")
)

// Size limits matching database CHECK constraints.
const (
	MaxSubjectLen = 1000
	MaxHTMLLen    = 256000
)

// keyPattern validates template key format: dot-separated lowercase segments.
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9_]*)+$`)

// ValidateKey checks if a template key matches the required format.
func ValidateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return nil
}

// Template represents a custom email template stored in the database.
type Template struct {
	ID              string    `json:"id"`
	TemplateKey     string    `json:"templateKey"`
	SubjectTemplate string    `json:"subjectTemplate"`
	HTMLTemplate    string    `json:"htmlTemplate"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// RenderedEmail holds the result of rendering a template.
type RenderedEmail struct {
	Subject string
	HTML    string
	Text    string
}

// EffectiveTemplate holds template source info for admin API responses.
type EffectiveTemplate struct {
	Source          string // "custom" or "builtin"
	TemplateKey     string
	SubjectTemplate string
	HTMLTemplate    string
	Enabled         bool
	Variables       []string // available variable names (for system keys)
}

// BuiltinTemplate holds a compiled built-in template and its default subject.
type BuiltinTemplate struct {
	SubjectTemplate string // raw subject template string
	HTMLTemplate    string // raw HTML template string
	Variables       []string
}

func DefaultBuiltins() map[string]BuiltinTemplate {
	systemVars, mfaVars := []string{"AppName", "ActionURL"}, []string{"AppName", "Code"}
	builtins := make(map[string]BuiltinTemplate, 5)

	keys := []struct {
		key     string
		subject string
		file    string
		vars    []string
	}{
		{"auth.password_reset", mailer.DefaultPasswordResetSubject, "password_reset.html", systemVars},
		{"auth.email_verification", mailer.DefaultVerificationSubject, "verification.html", systemVars},
		{"auth.magic_link", mailer.DefaultMagicLinkSubject, "magic_link.html", systemVars},
		{"auth.mfa_email_enroll", mailer.DefaultMFAEmailEnrollSubject, "mfa_email_enroll.html", mfaVars},
		{"auth.mfa_email_challenge", mailer.DefaultMFAEmailChallengeSubject, "mfa_email_challenge.html", mfaVars},
	}
	for _, k := range keys {
		html, err := mailer.BuiltinHTMLTemplate(k.file)
		if err != nil {
			// Embedded templates must always be available; panic on missing.
			panic(fmt.Sprintf("missing built-in email template %q: %v", k.file, err))
		}
		builtins[k.key] = BuiltinTemplate{SubjectTemplate: k.subject, HTMLTemplate: html, Variables: k.vars}
	}
	return builtins
}

// Service provides template rendering with fallback to built-in defaults.
type Service struct {
	store    TemplateStore
	builtins map[string]BuiltinTemplate
	mailer   mailer.Mailer
	logger   *slog.Logger
	mu       sync.RWMutex
}

// NewService creates a new template service.
func NewService(store TemplateStore, builtins map[string]BuiltinTemplate) *Service {
	return &Service{
		store:    store,
		builtins: builtins,
		logger:   slog.Default(),
	}
}

// SetLogger sets the logger for the service.
func (s *Service) SetLogger(l *slog.Logger) {
	s.logger = l
}

// List delegates to the store to list all custom overrides.
func (s *Service) List(ctx context.Context) ([]*Template, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.List(ctx)
}

// Upsert delegates to the store to create or update a custom template.
func (s *Service) Upsert(ctx context.Context, key, subjectTpl, htmlTpl string) (*Template, error) {
	if s.store == nil {
		return nil, errors.New("email template store not configured")
	}
	return s.store.Upsert(ctx, key, subjectTpl, htmlTpl)
}

// Delete delegates to the store to remove a custom template.
func (s *Service) Delete(ctx context.Context, key string) error {
	if s.store == nil {
		return errors.New("email template store not configured")
	}
	return s.store.Delete(ctx, key)
}

// SetEnabled delegates to the store to toggle the enabled flag.
func (s *Service) SetEnabled(ctx context.Context, key string, enabled bool) error {
	if s.store == nil {
		return errors.New("email template store not configured")
	}
	return s.store.SetEnabled(ctx, key, enabled)
}

// SystemKeys returns the list of built-in system template keys with their metadata.
// Keys are returned in sorted order for deterministic API responses.
func (s *Service) SystemKeys() []EffectiveTemplate {
	sortedKeys := make([]string, 0, len(s.builtins))
	for key := range s.builtins {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	keys := make([]EffectiveTemplate, 0, len(s.builtins))
	for _, key := range sortedKeys {
		b := s.builtins[key]
		keys = append(keys, EffectiveTemplate{
			Source:          "builtin",
			TemplateKey:     key,
			SubjectTemplate: b.SubjectTemplate,
			HTMLTemplate:    b.HTMLTemplate,
			Enabled:         true,
			Variables:       b.Variables,
		})
	}
	return keys
}

// SetMailer sets the mailer for sending emails.
func (s *Service) SetMailer(m mailer.Mailer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mailer = m
}

// renderTimeout is the maximum time allowed for template execution.
const renderTimeout = 5 * time.Second

// Render renders an email template by key, with fallback to built-in defaults.
func (s *Service) Render(ctx context.Context, key string, vars map[string]string) (*RenderedEmail, error) {
	// Set render timeout.
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()

	var customRenderErr error

	// Try custom override first (only if store is available).
	if s.store != nil {
		custom, err := s.store.Get(ctx, key)
		switch {
		case err == nil && custom.Enabled:
			rendered, renderErr := renderTemplates(ctx, key, custom.SubjectTemplate, custom.HTMLTemplate, vars)
			if renderErr == nil {
				return rendered, nil
			}
			customRenderErr = renderErr
			// Custom template failed — log and fall through to built-in.
			s.logger.Error("custom email template render failed, falling back to builtin",
				"key", key, "error", renderErr)
		case err == nil:
			// Disabled custom override; fall through to built-in.
		case errors.Is(err, ErrNotFound):
			// No custom override; fall through to built-in.
		default:
			return nil, fmt.Errorf("loading custom template %q: %w", key, err)
		}
	}

	// Try built-in template.
	builtin, ok := s.builtins[key]
	if !ok {
		if customRenderErr != nil {
			return nil, customRenderErr
		}
		return nil, fmt.Errorf("%w: %q", ErrNoTemplate, key)
	}

	return renderTemplates(ctx, key, builtin.SubjectTemplate, builtin.HTMLTemplate, vars)
}

// RenderWithFallback renders a template with graceful degradation: if custom
// template rendering fails, falls back to built-in. Only returns error if
// built-in also fails. Returns (subject, html, text, err) to satisfy
// auth.EmailTemplateRenderer without import coupling.
func (s *Service) RenderWithFallback(ctx context.Context, key string, vars map[string]string) (string, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()

	// Try custom override first (only if store is available).
	if s.store != nil {
		custom, err := s.store.Get(ctx, key)
		switch {
		case err == nil && custom.Enabled:
			rendered, renderErr := renderTemplates(ctx, key, custom.SubjectTemplate, custom.HTMLTemplate, vars)
			if renderErr == nil {
				return rendered.Subject, rendered.HTML, rendered.Text, nil
			}
			// Custom template failed — log and fall through to built-in.
			s.logger.Error("custom email template render failed, falling back to builtin",
				"key", key, "error", renderErr)
		case err == nil:
			// Disabled custom override; fall through to built-in.
		case errors.Is(err, ErrNotFound):
			// No custom override; fall through to built-in.
		default:
			// Store error (DB down, etc.) — log and fall through to built-in.
			s.logger.Error("failed to load custom email template, falling back to builtin",
				"key", key, "error", err)
		}
	}

	// Built-in fallback (should always succeed for system keys).
	builtin, ok := s.builtins[key]
	if !ok {
		return "", "", "", fmt.Errorf("%w: %q", ErrNoTemplate, key)
	}

	rendered, err := renderTemplates(ctx, key, builtin.SubjectTemplate, builtin.HTMLTemplate, vars)
	if err != nil {
		return "", "", "", err
	}
	return rendered.Subject, rendered.HTML, rendered.Text, nil
}

// GetEffective returns the active template source for a key.
func (s *Service) GetEffective(ctx context.Context, key string) (*EffectiveTemplate, error) {
	// Try custom override first (only if store is available).
	var custom *Template
	if s.store != nil {
		var err error
		custom, err = s.store.Get(ctx, key)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("getting custom template %q: %w", key, err)
			}
			custom = nil
		}
	}
	if custom != nil && custom.Enabled {
		et := &EffectiveTemplate{
			Source:          "custom",
			TemplateKey:     key,
			SubjectTemplate: custom.SubjectTemplate,
			HTMLTemplate:    custom.HTMLTemplate,
			Enabled:         custom.Enabled,
		}
		if b, ok := s.builtins[key]; ok {
			et.Variables = b.Variables
		}
		return et, nil
	}

	// Fall back to built-in.
	builtin, ok := s.builtins[key]
	if !ok {
		// Check if we have a disabled custom template.
		if custom != nil {
			return &EffectiveTemplate{
				Source:          "custom",
				TemplateKey:     key,
				SubjectTemplate: custom.SubjectTemplate,
				HTMLTemplate:    custom.HTMLTemplate,
				Enabled:         false,
			}, nil
		}
		return nil, fmt.Errorf("%w: %q", ErrNoTemplate, key)
	}

	return &EffectiveTemplate{
		Source:          "builtin",
		TemplateKey:     key,
		SubjectTemplate: builtin.SubjectTemplate,
		HTMLTemplate:    builtin.HTMLTemplate,
		Enabled:         true,
		Variables:       builtin.Variables,
	}, nil
}

// Preview renders provided template strings without saving them.
func (s *Service) Preview(ctx context.Context, key, subjectTpl, htmlTpl string, vars map[string]string) (*RenderedEmail, error) {
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()
	return renderTemplates(ctx, key, subjectTpl, htmlTpl, vars)
}

// Send renders a template and sends it via the mailer.
func (s *Service) Send(ctx context.Context, key, to string, vars map[string]string) error {
	s.mu.RLock()
	m := s.mailer
	s.mu.RUnlock()

	if m == nil {
		return errors.New("mailer not configured")
	}

	rendered, err := s.Render(ctx, key, vars)
	if err != nil {
		return err
	}

	return m.Send(ctx, &mailer.Message{
		To:      to,
		Subject: rendered.Subject,
		HTML:    rendered.HTML,
		Text:    rendered.Text,
	})
}
