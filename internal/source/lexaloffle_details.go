package source

import (
	"context"
	"io"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Details reads a cart's thread: the author, the text and images of the
// first post, and its tags. The cart's 128x128 label, a screenshot its
// author picked, comes first among the images.
func (s *lexaloffle) Details(ctx context.Context, g Game) (*Details, error) {
	resp, err := get(ctx, s.client, g.Info, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	doc, err := html.Parse(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return parsePico8Thread(doc, resp.Request.URL), nil
}

func parsePico8Thread(doc *html.Node, base *url.URL) *Details {
	d := &Details{}
	var body *html.Node
	walk(doc, func(n *html.Node) bool {
		switch {
		case n.DataAtom == atom.Img && len(d.Images) == 0 && strings.HasPrefix(attr(n, "src"), "/bbs/thumbs/"):
			// The label, shown by the first post's dormant player.
			if u, err := base.Parse(attr(n, "src")); err == nil {
				d.Images = append(d.Images, u.String())
			}
		case n.DataAtom == atom.A && d.Author == "" && strings.HasPrefix(attr(n, "href"), "/bbs/?uid="):
			// The poster's name is the first bold user link.
			if b := firstChild(n, atom.B); b != nil {
				d.Author = strings.TrimSpace(textOf(b))
			}
		case n.DataAtom == atom.Div && body == nil && strings.ReplaceAll(attr(n, "style"), " ", "") == "min-height:44px;":
			body = n // walked on: the label is inside
		case n.DataAtom == atom.Span && attr(n, "class") == "tag":
			d.Tags = append(d.Tags, strings.TrimSpace(textOf(n)))
		}
		return true
	})
	if body == nil {
		return d
	}

	var b strings.Builder
	seen := map[string]bool{}
	var visit func(n *html.Node)
	visit = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(spaces.ReplaceAllString(n.Data, " "))
			return
		case html.ElementNode:
		default:
			return
		}
		switch n.DataAtom {
		case atom.Script, atom.Style, atom.Textarea, atom.Iframe, atom.Canvas, atom.Noscript:
			return
		case atom.Br:
			b.WriteByte('\n')
			return
		case atom.Img:
			if u, err := base.Parse(attr(n, "src")); err == nil && (u.Scheme == "http" || u.Scheme == "https") &&
				!strings.HasPrefix(u.Path, "/gfx/") && !seen[u.String()] {
				seen[u.String()] = true
				d.Images = append(d.Images, u.String())
			}
			return
		}
		// Skip the embedded player and hidden panels.
		if strings.Contains(strings.ReplaceAll(attr(n, "style"), " ", ""), "display:none") ||
			strings.HasPrefix(attr(n, "class"), "playarea") {
			return
		}
		var block bool
		switch n.DataAtom {
		case atom.P, atom.Div, atom.Li, atom.Ul, atom.Ol, atom.Pre, atom.Blockquote, atom.Tr,
			atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
			block = true
		}
		if block {
			b.WriteByte('\n')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
		if block {
			b.WriteByte('\n')
		}
	}
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		visit(c)
	}
	d.Description = tidyText(b.String())
	return d
}

var (
	spaces     = regexp.MustCompile(`\s+`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

// tidyText trims every line and keeps at most one blank line in a row.
func tidyText(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	s = blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.TrimSpace(s)
}

// walk visits n and its descendants depth first; f returns false to skip
// a node's children.
func walk(n *html.Node, f func(*html.Node) bool) {
	if n.Type == html.ElementNode && !f(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func firstChild(n *html.Node, a atom.Atom) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.DataAtom == a {
			return c
		}
	}
	return nil
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return b.String()
}
