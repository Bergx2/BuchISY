package i18n

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"testing"

	"github.com/bergx2/buchisy/assets"
)

// TestEmbeddedTranslationsParse guards against a UTF-8 BOM (or any other
// corruption) sneaking into a translation file — which would make
// json.Unmarshal fail and the whole UI fall back to raw keys.
func TestEmbeddedTranslationsParse(t *testing.T) {
	for _, name := range []string{"i18n/de.json", "i18n/en.json"} {
		data, err := fs.ReadFile(assets.Translations, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
			t.Errorf("%s starts with a UTF-8 BOM — strip it (breaks json.Unmarshal)", name)
		}
		m := map[string]string{}
		if err := json.Unmarshal(stripBOM(data), &m); err != nil {
			t.Fatalf("%s does not parse as JSON: %v", name, err)
		}
		if m["app.title"] == "" {
			t.Errorf("%s missing the app.title key (loaded %d keys)", name, len(m))
		}
	}
}
