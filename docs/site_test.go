// Package docs holds the GitHub Pages site; the test keeps its translations
// in step with the English page.
package docs

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// pageStrings returns the English text of every translatable element and
// attribute in index.html, keyed like the locale files.
func pageStrings(t *testing.T) map[string]string {
	f, err := os.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := html.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	attr := func(n *html.Node, name string) (string, bool) {
		for _, a := range n.Attr {
			if a.Key == name {
				return a.Val, true
			}
		}
		return "", false
	}
	add := func(m map[string]string, key, val string) {
		if old, ok := m[key]; ok && old != val {
			t.Errorf("key %q used for different texts: %q and %q", key, old, val)
		}
		m[key] = val
	}
	m := map[string]string{}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		if key, ok := attr(n, "data-i18n"); ok {
			var b strings.Builder
			for c := range n.ChildNodes() {
				html.Render(&b, c)
			}
			add(m, key, b.String())
		}
		if key, ok := attr(n, "data-i18n-alt"); ok {
			v, _ := attr(n, "alt")
			add(m, key, v)
		}
		if key, ok := attr(n, "data-i18n-label"); ok {
			v, _ := attr(n, "aria-label")
			add(m, key, v)
		}
	}
	return m
}

var tagRE = regexp.MustCompile(`<[^>]+>`)

// tags lists the markup of s so translations keep the same code, links and
// emphasis as the English text.
func tags(s string) []string {
	ts := tagRE.FindAllString(s, -1)
	slices.Sort(ts)
	return ts
}

func TestLocalesComplete(t *testing.T) {
	en := pageStrings(t)
	sel := regexp.MustCompile(`<option value="([^"]+)"`)
	page, _ := os.ReadFile("index.html")
	opts := sel.FindAllStringSubmatch(string(page), -1)
	if len(opts) < 2 {
		t.Fatal("language picker not found")
	}
	for _, o := range opts[1:] {
		lang := o[1]
		b, err := os.ReadFile("locales/" + lang + ".json")
		if err != nil {
			t.Errorf("%s: %v", lang, err)
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(b, &m); err != nil {
			t.Errorf("%s: %v", lang, err)
			continue
		}
		for k, v := range en {
			tr, ok := m[k]
			switch {
			case !ok || tr == "":
				t.Errorf("%s: missing translation for %q", lang, k)
			case !slices.Equal(tags(tr), tags(v)):
				t.Errorf("%s: %q changes markup: %q", lang, k, tr)
			}
		}
		for k := range m {
			if _, ok := en[k]; !ok {
				t.Errorf("%s: unused translation %q", lang, k)
			}
		}
	}
}
