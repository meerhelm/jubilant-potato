package source

import (
	"cmp"
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
)

// PICO-8 carts from the Lexaloffle BBS: the featured list plus the newest
// releases of the Cartridges subforum, fetched as .p8.png files like
// SPLORE does.

// lexaloffleBBS is the BBS root; tests point it at a fake server.
var lexaloffleBBS = "https://www.lexaloffle.com/bbs"

// Listing pages of 30 carts each.
var lexaloffleLists = []struct {
	order string
	pages int
}{
	{"featured", 20}, // ~15 pages exist; stops at the first empty one
	{"ts", 3},        // newest
}

type lexaloffle struct {
	cfg    config.Source
	client *http.Client
}

func newLexaloffle(c config.Source, client *http.Client) *lexaloffle {
	return &lexaloffle{cfg: c, client: client}
}

func (s *lexaloffle) Name() string { return s.cfg.Name }
func (s *lexaloffle) Kind() string { return "PICO-8" }

func (s *lexaloffle) Systems(context.Context) ([]System, error) {
	sys, _ := platform.SystemByID("pico8")
	return []System{{ID: sys.ID, Key: sys.ID, Label: sys.Name}}, nil
}

// pico8Cart is one row of the BBS listing.
type pico8Cart struct {
	tid   int
	title string
	lid   string // cart id: "celeste-3", or a number for old carts
}

func (s *lexaloffle) Games(ctx context.Context, sys System) ([]Game, error) {
	type page struct {
		carts []pico8Cart
		err   error
	}
	var mu sync.Mutex
	pages := map[string][]page{}
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4) // be gentle with the BBS
	for _, l := range lexaloffleLists {
		pages[l.order] = make([]page, l.pages) // before any fetch writes to it
	}
	for _, l := range lexaloffleLists {
		for i := range l.pages {
			wg.Add(1)
			go func() {
				defer wg.Done()
				slots <- struct{}{}
				defer func() { <-slots }()
				carts, err := s.list(ctx, l.order, i+1)
				mu.Lock()
				pages[l.order][i] = page{carts, err}
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	seen := map[int]bool{}
	files := map[string]bool{}
	var out []Game
	var firstErr error
	for _, l := range lexaloffleLists {
		for _, p := range pages[l.order] {
			if p.err != nil {
				firstErr = cmp.Or(firstErr, p.err)
				break
			}
			if len(p.carts) == 0 {
				break // past the last page
			}
			for _, c := range p.carts {
				if seen[c.tid] {
					continue
				}
				seen[c.tid] = true
				base := cmp.Or(pico8FileName(c.title), c.lid)
				file := base + ".p8.png"
				if files[strings.ToLower(file)] {
					file = base + " " + c.lid + ".p8.png"
				}
				files[strings.ToLower(file)] = true
				out = append(out, Game{
					Name: c.title, Title: c.title, File: file,
					Group:  "pico8:" + strconv.Itoa(c.tid),
					URL:    lexaloffleBBS + pico8CartPath(c.lid),
					Info:   lexaloffleBBS + "/?tid=" + strconv.Itoa(c.tid),
					System: sys.ID,
				})
			}
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	sortGames(out)
	return out, nil
}

// list fetches one page of the Cartridges subforum (cat 7, sub 2).
func (s *lexaloffle) list(ctx context.Context, order string, page int) ([]pico8Cart, error) {
	u := fmt.Sprintf("%s/lister.php?cat=7&sub=2&mode=carts&orderby=%s&page=%d", lexaloffleBBS, order, page)
	resp, err := get(ctx, s.client, u, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parsePico8List(string(b)), nil
}

// A listing row is a JS array: ['pid', tid, `title`, "thumb", ...].
var pico8Row = regexp.MustCompile("\\[\\s*'\\d+',\\s*(\\d+),\\s*`([^`]*)`,\\s*\"([^\"]*)\"")

// Thumbnails are named after the cart: pico8_<lid>.png, or pico<lid>.png
// for old numeric ids.
var pico8Thumb = regexp.MustCompile(`/thumbs/pico(?:8_([A-Za-z0-9_-]+)|(\d+))\.png$`)

// parsePico8List extracts the carts from the listing's pdat array.
func parsePico8List(page string) []pico8Cart {
	i := strings.Index(page, "pdat=[")
	if i < 0 {
		return nil
	}
	page = page[i:]
	if j := strings.Index(page, "\n];"); j >= 0 {
		page = page[:j]
	} else if j := strings.Index(page, "];"); j >= 0 {
		page = page[:j]
	}
	var out []pico8Cart
	for _, m := range pico8Row.FindAllStringSubmatch(page, -1) {
		tid, _ := strconv.Atoi(m[1])
		t := pico8Thumb.FindStringSubmatch(m[3])
		if tid == 0 || t == nil {
			continue // thread without a cart
		}
		lid := t[1] + t[2]
		title := strings.TrimSpace(html.UnescapeString(m[2]))
		if title == "" {
			title = lid
		}
		out = append(out, pico8Cart{tid: tid, title: title, lid: lid})
	}
	return out
}

// pico8CartPath mirrors the BBS's get_cart_url for PICO-8.
func pico8CartPath(lid string) string {
	if n, err := strconv.Atoi(lid); err == nil {
		return fmt.Sprintf("/cposts/%d/%s.p8.png", n/10000, lid)
	}
	return fmt.Sprintf("/cposts/%s/%s.p8.png", lid[:min(2, len(lid))], lid)
}

// pico8FileName makes a cart title safe as a file name on FAT and exFAT.
func pico8FileName(title string) string {
	name := strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune(`/\:*?"<>|`, r) {
			return ' '
		}
		return r
	}, title)
	name = strings.Join(strings.Fields(name), " ")
	if r := []rune(name); len(r) > 100 {
		name = string(r[:100])
	}
	return strings.TrimRight(name, ". ")
}
