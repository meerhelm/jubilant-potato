package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// uiStrings collects every English string passed to T() in this package,
// plus the few that reach T() through variables.
func uiStrings(t *testing.T) map[string]bool {
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`T\(("(?:[^"\\]|\\.)*")`)
	keys := map[string]bool{
		// keyboard special keys, download states and the no-package error
		keySpace: true, keyDel: true, keyClear: true, keyDone: true,
		string(errNoPackage): true,
	}
	for _, s := range stateNames {
		keys[s] = true
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			s, err := strconv.Unquote(m[1])
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			keys[s] = true
		}
	}
	return keys
}

func verbs(s string) string {
	return strings.Join(regexp.MustCompile(`%[sd]`).FindAllString(s, -1), "")
}

func TestLocalesComplete(t *testing.T) {
	keys := uiStrings(t)
	for _, l := range Languages[1:] {
		b, err := localeFS.ReadFile("locales/" + l.Code + ".json")
		if err != nil {
			t.Errorf("%s: %v", l.Code, err)
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(b, &m); err != nil {
			t.Errorf("%s: %v", l.Code, err)
			continue
		}
		for k := range keys {
			tr, ok := m[k]
			switch {
			case !ok || tr == "":
				t.Errorf("%s: missing translation for %q", l.Code, k)
			case verbs(tr) != verbs(k):
				t.Errorf("%s: %q changes format verbs: %q", l.Code, k, tr)
			}
		}
		for k := range m {
			if !keys[k] {
				t.Errorf("%s: unused translation %q", l.Code, k)
			}
		}
	}
}

func TestResolveLanguage(t *testing.T) {
	t.Setenv("LANG", "zh_CN.UTF-8")
	for _, c := range []struct{ configured, firmware, want string }{
		{"pl", "ru_RU", "pl"},
		{"", "uk_UA", "uk"},
		{"", "zh_TW", "zh-Hant"},
		{"", "pt_BR", "pt"},
		{"", "", "zh-Hans"}, // from $LANG
		{"", "de_DE", "zh-Hans"},
		{"xx", "", "zh-Hans"},
	} {
		if got := resolveLanguage(c.configured, c.firmware); got != c.want {
			t.Errorf("resolveLanguage(%q, %q) = %q, want %q", c.configured, c.firmware, got, c.want)
		}
	}
	setLanguage("be")
	defer setLanguage("en")
	if T("Downloads") != "Спампоўкі" || T("%d games", 3) != "Гульняў: 3" {
		t.Errorf("Belarusian not applied: %q", T("Downloads"))
	}
}
