package bottle

import (
	"strings"
	"testing"
)

// update.js holds pages with work in them, so the one property that matters
// is that it reloads only from the notice's button, and only after asking
// the server again for what the page loaded.
func TestUpdateReloadsOnlyWhenAsked(t *testing.T) {
	js := string(UpdateJS())
	if n := strings.Count(js, "location.reload()"); n != 1 {
		t.Fatalf("update.js reloads in %d places; want one, the Reload button's", n)
	}
	for _, s := range []string{"cache: 'no-cache'", "'no-store'", "data-stamp"} {
		if !strings.Contains(js, s) {
			t.Errorf("update.js lacks %s", s)
		}
	}
}
