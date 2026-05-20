package emailtemplates

import (
	"context"
	"fmt"
	"html"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

// renderTemplates parses and executes subject + HTML templates against vars.
func renderTemplates(ctx context.Context, key, subjectTpl, htmlTpl string, vars map[string]string) (*RenderedEmail, error) {
	st, err := parseSubject(key, subjectTpl)
	if err != nil {
		return nil, fmt.Errorf("%w: subject: %v", ErrParseFailed, err)
	}
	ht, err := parseHTML(key, htmlTpl)
	if err != nil {
		return nil, fmt.Errorf("%w: html: %v", ErrParseFailed, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRenderFailed, err)
	}

	var subBuf strings.Builder
	if err := st.Execute(&subBuf, vars); err != nil {
		return nil, fmt.Errorf("%w: subject: %v", ErrRenderFailed, err)
	}
	var htmlBuf strings.Builder
	if err := ht.Execute(&htmlBuf, vars); err != nil {
		return nil, fmt.Errorf("%w: html: %v", ErrRenderFailed, err)
	}

	htmlBody := htmlBuf.String()
	return &RenderedEmail{
		Subject: subBuf.String(),
		HTML:    htmlBody,
		Text:    stripHTML(htmlBody),
	}, nil
}

// parseSubject parses a subject template string with missingkey=error.
func parseSubject(key, tpl string) (*texttemplate.Template, error) {
	return texttemplate.New(key + ".subject").
		Option("missingkey=error").
		Parse(tpl)
}

// parseHTML parses an HTML template string with missingkey=error and empty FuncMap.
func parseHTML(key, tpl string) (*htmltemplate.Template, error) {
	return htmltemplate.New(key + ".html").
		Option("missingkey=error").
		Funcs(htmltemplate.FuncMap{}).
		Parse(tpl)
}

// stripHTML removes HTML tags, decodes HTML entities, and collapses whitespace
// for plaintext fallback.
func stripHTML(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	decoded := html.UnescapeString(out.String())
	lines := strings.Split(decoded, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return strings.Join(result, "\n")
}
