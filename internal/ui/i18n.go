package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Translations are keyed by the English UI string. Add a language by
// dropping locales/<code>.json next to the others and listing it below.
//
//go:embed locales/*.json
var localeFS embed.FS

// Language is a UI language with its name in that language.
type Language struct {
	Code, Name string
}

// Languages in the order the picker shows them.
var Languages = []Language{
	{"en", "English"},
	{"ru", "Русский"},
	{"uk", "Українська"},
	{"be", "Беларуская"},
	{"pl", "Polski"},
	{"es", "Español"},
	{"pt", "Português"},
	{"zh-Hans", "简体中文"},
	{"zh-Hant", "繁體中文"},
}

var (
	lang         = "en"
	translations map[string]string // for lang; nil for English
)

// normalizeLanguage maps settings like "ru_RU", "pt-BR" or "zh_TW" to a
// supported code, or "" if there is none.
func normalizeLanguage(l string) string {
	l = strings.ToLower(strings.TrimSpace(l))
	l, _, _ = strings.Cut(l, ".") // "ru_RU.UTF-8"
	l = strings.ReplaceAll(l, "_", "-")
	switch {
	case l == "":
		return ""
	case l == "zh-hant" || strings.HasPrefix(l, "zh-tw") || strings.HasPrefix(l, "zh-hk") || strings.HasPrefix(l, "zh-mo"):
		return "zh-Hant"
	case strings.HasPrefix(l, "zh"):
		return "zh-Hans"
	}
	base, _, _ := strings.Cut(l, "-")
	for _, x := range Languages {
		if x.Code == base {
			return base
		}
	}
	return ""
}

// resolveLanguage picks the UI language: the user's setting, then the
// firmware's, then $LANG, else English.
func resolveLanguage(configured, firmware string) string {
	for _, l := range []string{configured, firmware, os.Getenv("LANG")} {
		if code := normalizeLanguage(l); code != "" {
			return code
		}
	}
	return "en"
}

// setLanguage switches the UI language; unknown codes fall back to English.
func setLanguage(code string) {
	lang, translations = "en", nil
	if code == "" || code == "en" {
		return
	}
	b, err := localeFS.ReadFile("locales/" + code + ".json")
	if err != nil {
		return
	}
	var m map[string]string
	if json.Unmarshal(b, &m) == nil {
		lang, translations = code, m
	}
}

// languageName returns the native name of a language code.
func languageName(code string) string {
	for _, l := range Languages {
		if l.Code == code {
			return l.Name
		}
	}
	return code
}

// T returns the translation of an English UI string.
func T(s string, args ...any) string {
	if tr, ok := translations[s]; ok {
		s = tr
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
