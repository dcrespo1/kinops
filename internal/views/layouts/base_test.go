package layouts

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestBaseIncludesAccessibleResponsiveNavigation(t *testing.T) {
	var output bytes.Buffer
	content := templ.ComponentFunc(func(context.Context, io.Writer) error { return nil })
	if err := Base("KinOps", content).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, expected := range []string{
		`class="skip-link"`,
		`href="/static/css/app.css?v=20260803-4"`,
		`href="#main-content"`,
		`class="primary-nav"`,
		`class="primary-links desktop-links"`,
		`class="nav-menu"`,
		`class="nav-toggle"`,
		`class="primary-links mobile-links"`,
		`id="main-content" tabindex="-1"`,
		`aria-live="polite"`,
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("rendered layout missing %s", expected)
		}
	}
}
