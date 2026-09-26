// Package romname parses ROM file names in the No-Intro, Redump and GoodTools
// conventions, e.g. "Legend of Zelda, The (USA, Europe) (Rev 1).zip" or
// "Chrono Trigger (U) [T+Rus_Chief-Net].smc".
package romname

import (
	"path"
	"strconv"
	"strings"
)

// Kind ranks how "official" a dump is; lower is better.
type Kind int

const (
	Retail      Kind = iota
	Translation      // fan translation
	Hack             // ROM hack or other modification
	Prerelease       // beta, prototype, demo, sample, debug, kiosk...
	Bad              // known bad or overdumped
)

// Info describes one file name.
type Info struct {
	Title     string   // name without tags or extension
	Regions   []string // canonical region names, e.g. "USA", "Europe"
	Languages []string // lower-case ISO 639-1 codes; inferred from regions when not tagged
	Revision  int      // 0 for the original release, higher is newer
	Kind      Kind
	Verified  bool // GoodTools [!]
}

// Parse extracts the title and tags from a file name.
func Parse(file string) Info {
	name := strings.TrimSuffix(file, path.Ext(file))
	var info Info
	var explicitLangs bool

	// The title runs up to the first tag.
	cut := len(name)
	if i := strings.IndexAny(name, "(["); i > 0 {
		cut = i
	}
	info.Title = strings.TrimSpace(name[:cut])
	if info.Title == "" {
		info.Title = name
	}

	for _, tag := range tags(name[cut:]) {
		square := tag[0] == '['
		body := strings.TrimSpace(tag[1 : len(tag)-1])
		if square {
			parseSquare(body, &info, &explicitLangs)
			continue
		}
		if regions, ok := parseRegions(body); ok {
			info.Regions = append(info.Regions, regions...)
			continue
		}
		if langs, ok := parseLanguages(body); ok {
			info.Languages = append(info.Languages, langs...)
			explicitLangs = true
			continue
		}
		parseParen(body, &info)
	}

	if !explicitLangs {
		for _, r := range info.Regions {
			if l := regionLanguage[r]; l != "" && !contains(info.Languages, l) {
				info.Languages = append(info.Languages, l)
			}
		}
	}
	return info
}

// tags splits "(USA) (Rev 1) [!]" into its bracketed groups.
func tags(s string) []string {
	var out []string
	for {
		i := strings.IndexAny(s, "([")
		if i < 0 {
			return out
		}
		closer := ")"
		if s[i] == '[' {
			closer = "]"
		}
		j := strings.Index(s[i:], closer)
		if j < 0 {
			return out
		}
		out = append(out, s[i:i+j+1])
		s = s[i+j+1:]
	}
}

func parseParen(body string, info *Info) {
	lower := strings.ToLower(body)
	switch {
	case strings.HasPrefix(lower, "rev "):
		info.Revision = max(info.Revision, revision(body[4:]))
	case len(lower) > 1 && lower[0] == 'v' && (lower[1] >= '0' && lower[1] <= '9'):
		info.Revision = max(info.Revision, revision(body[1:]))
	case lower == "hack" || strings.HasSuffix(lower, " hack"):
		info.Kind = max(info.Kind, Hack)
	case strings.HasPrefix(lower, "beta"), strings.HasPrefix(lower, "proto"),
		strings.HasPrefix(lower, "demo"), strings.HasPrefix(lower, "sample"),
		strings.HasPrefix(lower, "debug"), strings.HasPrefix(lower, "kiosk"),
		strings.HasPrefix(lower, "alpha"), strings.HasPrefix(lower, "preview"),
		lower == "program", lower == "pirate", lower == "competition cart":
		info.Kind = max(info.Kind, Prerelease)
	}
}

// parseSquare handles GoodTools flags like [!], [b1], [h2C], [T+Rus].
func parseSquare(body string, info *Info, explicitLangs *bool) {
	if body == "" {
		return
	}
	switch {
	case body == "!":
		info.Verified = true
	case body[0] == 'T' && len(body) > 1 && (body[1] == '+' || body[1] == '-'):
		info.Kind = max(info.Kind, Translation)
		code := strings.ToLower(body[2:])
		if i := strings.IndexAny(code, "_ 0123456789"); i >= 0 {
			code = code[:i]
		}
		if l := translationLanguage[code]; l != "" {
			// A translation's language replaces the original's.
			if !*explicitLangs {
				info.Languages = nil
				*explicitLangs = true
			}
			info.Languages = append(info.Languages, l)
		}
	case body[0] == 'b' || body[0] == 'o':
		if len(body) == 1 || isDigits(body[1:]) {
			info.Kind = max(info.Kind, Bad)
		}
	case body[0] == 'h' || body[0] == 'p' || body[0] == 't' || body[0] == 'f':
		// hack, pirate, trainer, fixed
		info.Kind = max(info.Kind, Hack)
	}
}

func parseRegions(body string) ([]string, bool) {
	// No-Intro full names: (USA), (USA, Europe).
	var out []string
	for _, part := range strings.Split(body, ",") {
		r, ok := regionNames[strings.TrimSpace(part)]
		if !ok {
			out = nil
			break
		}
		out = append(out, r)
	}
	if len(out) > 0 {
		return out, true
	}
	// GoodTools codes, possibly combined: (U), (JU), (UE), (HK).
	if codes, ok := goodRegions[body]; ok {
		return codes, true
	}
	if len(body) <= 3 && isUpper(body) {
		for _, c := range body {
			r, ok := goodRegions[string(c)]
			if !ok {
				return nil, false
			}
			out = append(out, r...)
		}
		return out, true
	}
	return nil, false
}

func parseLanguages(body string) ([]string, bool) {
	var out []string
	for _, part := range strings.Split(body, ",") {
		part = strings.TrimSpace(part)
		// No-Intro uses "En", "Fr", and region-specific forms like "Pt-BR".
		base, _, _ := strings.Cut(part, "-")
		if len(base) != 2 || base[0] < 'A' || base[0] > 'Z' || base[1] < 'a' || base[1] > 'z' {
			return nil, false
		}
		out = append(out, strings.ToLower(base))
	}
	return out, len(out) > 0
}

// revision turns "1", "A", "1.1" or "02" into a comparable number.
func revision(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if c := s[0]; c >= 'A' && c <= 'Z' && len(s) == 1 {
		return int(c-'A') + 1
	}
	major, minor, _ := strings.Cut(s, ".")
	n, err := strconv.Atoi(major)
	if err != nil {
		return 0
	}
	m, _ := strconv.Atoi(minor)
	return n*100 + m
}

var regionNames = map[string]string{
	"USA": "USA", "Europe": "Europe", "Japan": "Japan", "World": "World",
	"Asia": "Asia", "Australia": "Australia", "Brazil": "Brazil", "Canada": "Canada",
	"China": "China", "France": "France", "Germany": "Germany", "Hong Kong": "Hong Kong",
	"Italy": "Italy", "Korea": "Korea", "Netherlands": "Netherlands", "Spain": "Spain",
	"Sweden": "Sweden", "Taiwan": "Taiwan", "UK": "UK", "Russia": "Russia",
	"Scandinavia": "Scandinavia", "Latin America": "Latin America", "Poland": "Poland",
	"Portugal": "Portugal", "Greece": "Greece", "Denmark": "Denmark", "Finland": "Finland",
	"Norway": "Norway", "Mexico": "Mexico", "Argentina": "Argentina", "Unknown": "Unknown",
}

var goodRegions = map[string][]string{
	"U": {"USA"}, "E": {"Europe"}, "J": {"Japan"}, "W": {"World"}, "R": {"Russia"},
	"G": {"Germany"}, "F": {"France"}, "S": {"Spain"}, "I": {"Italy"}, "K": {"Korea"},
	"B": {"Brazil"}, "A": {"Australia"}, "C": {"China"}, "Ch": {"China"},
	"UK": {"UK"}, "HK": {"Hong Kong"}, "NL": {"Netherlands"}, "Sw": {"Sweden"},
}

var regionLanguage = map[string]string{
	"USA": "en", "Europe": "en", "World": "en", "UK": "en", "Australia": "en", "Canada": "en",
	"Japan": "ja", "Russia": "ru", "Germany": "de", "France": "fr", "Spain": "es",
	"Italy": "it", "Brazil": "pt", "Portugal": "pt", "Korea": "ko", "China": "zh",
	"Taiwan": "zh", "Hong Kong": "zh", "Netherlands": "nl", "Sweden": "sv", "Poland": "pl",
	"Latin America": "es", "Mexico": "es", "Argentina": "es",
}

var translationLanguage = map[string]string{
	"rus": "ru", "eng": "en", "spa": "es", "fre": "fr", "fra": "fr", "ger": "de", "deu": "de",
	"ita": "it", "por": "pt", "bra": "pt", "pol": "pl", "chi": "zh", "kor": "ko", "jap": "ja",
	"ukr": "uk", "dut": "nl", "swe": "sv", "gre": "el", "tur": "tr", "cat": "ca", "hun": "hu",
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

func isUpper(s string) bool {
	for _, c := range s {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return s != ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
