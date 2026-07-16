package server

import (
	"strings"
	"testing"
)

func TestRewriteAdminIndexHTMLUsesCustomAdminPathForAbsoluteEntryAssets(t *testing.T) {
	html := strings.Join([]string{
		`<script type="module" crossorigin src="/admin/assets/index-abc123.js"></script>`,
		`<link rel="stylesheet" crossorigin href="/admin/assets/index-def456.css">`,
		`<link rel="modulepreload" crossorigin href="/assets/vendor-ghi789.js">`,
		`<style>.logo{background:url(/admin/assets/logo.png)}.font{src:url('/assets/font.woff2')}</style>`,
	}, "\n")

	rewritten := rewriteAdminIndexHTML(html, "/dashboard")

	for _, expected := range []string{
		`src="/dashboard/assets/index-abc123.js"`,
		`href="/dashboard/assets/index-def456.css"`,
		`href="/dashboard/assets/vendor-ghi789.js"`,
		`url(/dashboard/assets/logo.png)`,
		`url('/dashboard/assets/font.woff2')`,
	} {
		if !strings.Contains(rewritten, expected) {
			t.Fatalf("rewriteAdminIndexHTML() missing %q in:\n%s", expected, rewritten)
		}
	}

	if strings.Contains(rewritten, `"/admin/assets/`) || strings.Contains(rewritten, `"/assets/`) {
		t.Fatalf("rewriteAdminIndexHTML() left an absolute default asset path in:\n%s", rewritten)
	}
}
