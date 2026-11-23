package main

import (
	"fmt"
	"hash/fnv"
	"html"
	"net/http"
	"strings"
)

// Homepage. GET /api/home returns the ordered list of blocks (from the home_blocks
// table). Each block names its own "source" endpoint; the frontend fetches each
// block independently and renders whatever comes back. Nothing about the homepage's
// content or order is hardcoded in the frontend.

type HomeBlock struct {
	ID     int    `json:"id"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Source string `json:"source"`
}

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id, kind, title, source FROM home_blocks WHERE enabled = 1 ORDER BY position`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	blocks := []HomeBlock{}
	for rows.Next() {
		var b HomeBlock
		if err := rows.Scan(&b.ID, &b.Kind, &b.Title, &b.Source); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		blocks = append(blocks, b)
	}
	writeJSON(w, 200, map[string]any{"blocks": blocks})
}

type Banner struct {
	ID       int    `json:"id"`
	Headline string `json:"headline"`
	Subline  string `json:"subline"`
	CtaLabel string `json:"ctaLabel"`
	CtaLink  string `json:"ctaLink"`
	ImageURL string `json:"imageUrl"`
	Hue      int    `json:"hue"`
}

// Banner service: reads its own banners table.
func (a *App) blockBanners(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT b.id, b.headline, b.subline, b.cta_label, b.cta_link, COALESCE(p.slug, ''), b.hue
		FROM banners b LEFT JOIN products p ON p.id = b.product_id ORDER BY b.position`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []Banner{}
	for rows.Next() {
		var b Banner
		var slug string
		if err := rows.Scan(&b.ID, &b.Headline, &b.Subline, &b.CtaLabel, &b.CtaLink, &slug, &b.Hue); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if slug != "" {
			b.ImageURL = "/api/covers/" + slug
		}
		out = append(out, b)
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

type Category struct {
	Label string `json:"label"`
	Link  string `json:"link"`
	Hue   int    `json:"hue"`
	Count int    `json:"count"`
}

// Category service: reads its own categories table and counts listings live.
func (a *App) blockCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT label, link, hue FROM categories ORDER BY position`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.Label, &c.Link, &c.Hue); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, c)
	}
	rows.Close()
	for i := range out {
		col, val := categoryFilter(out[i].Link)
		if col != "" {
			a.db.QueryRow(`SELECT COUNT(*) FROM products WHERE `+col+` = ?`, val).Scan(&out[i].Count)
		}
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func categoryFilter(link string) (string, string) {
	_, qs, ok := strings.Cut(link, "?")
	if !ok {
		return "", ""
	}
	k, v, _ := strings.Cut(qs, "=")
	v = strings.ReplaceAll(v, "+", " ")
	switch k {
	case "platform", "type", "genre":
		return k, v
	}
	return "", ""
}

// Product rails. Each is its own query ("service") over the catalog.
// One listing per title (the cheapest edition), so rails don't show the same game 3x.
func (a *App) rail(w http.ResponseWriter, where, order string, limit int) {
	q := listingCTE + `, ranked AS (
		SELECT *, ROW_NUMBER() OVER (PARTITION BY title ORDER BY min_price) AS rn FROM listing WHERE min_price IS NOT NULL
	) SELECT ` + listingCols + ` FROM ranked WHERE rn = 1 AND ` + where + ` ORDER BY ` + order + ` LIMIT ` + fmt.Sprint(limit)
	rows, err := a.db.Query(q)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	items, err := scanSummaries(rows)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// Best sellers: seeded popularity plus real purchases made through checkout.
func (a *App) blockBestsellers(w http.ResponseWriter, r *http.Request) {
	a.rail(w, "type IN ('Game','DLC')", "sales_count DESC", 12)
}

func (a *App) blockDeals(w http.ResponseWriter, r *http.Request) {
	a.rail(w, "retail > min_price", "(retail - min_price) * 1.0 / retail DESC", 12)
}

func (a *App) blockNewReleases(w http.ResponseWriter, r *http.Request) {
	a.rail(w, "type IN ('Game','DLC')", "release_year DESC, sales_count DESC", 12)
}

func (a *App) blockBudget(w http.ResponseWriter, r *http.Request) {
	a.rail(w, "min_price <= 1000 AND type = 'Game'", "sales_count DESC", 12)
}

// GET /api/covers/{slug} : generated SVG cover art. We don't ship publisher artwork
// (licensing), so every listing gets a deterministic abstract cover instead.
func (a *App) handleCover(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var title, platform, typ string
	var hue int
	if err := a.db.QueryRow(`SELECT title, platform, type, cover_hue FROM products WHERE slug = ?`, slug).Scan(&title, &platform, &typ, &hue); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(coverSVG(title, platform, typ, hue)))
}

func coverSVG(title, platform, typ string, hue int) string {
	h := fnv.New32a()
	h.Write([]byte(title))
	seed := h.Sum32()
	h2 := (hue + 40 + int(seed%60)) % 360

	var shapes strings.Builder
	for i := 0; i < 6; i++ {
		s := (seed >> (i * 5)) & 0x1f
		cx := 20 + int(s)*9%260
		cy := 30 + int(s*7)%300
		rad := 30 + int(s*3)%90
		fmt.Fprintf(&shapes, `<circle cx="%d" cy="%d" r="%d" fill="hsl(%d 90%% 70%%)" opacity="0.%d"/>`, cx, cy, rad, (h2+i*25)%360, 10+int(s%15))
	}

	// wrap title onto up to 4 lines of ~13 chars
	words := strings.Fields(title)
	var lines []string
	cur := ""
	for _, wd := range words {
		if len(cur)+len(wd)+1 > 13 && cur != "" {
			lines = append(lines, cur)
			cur = wd
		} else if cur == "" {
			cur = wd
		} else {
			cur += " " + wd
		}
	}
	lines = append(lines, cur)
	if len(lines) > 4 {
		lines = append(lines[:3], strings.Join(lines[3:], " "))
	}
	var text strings.Builder
	y := 300 - len(lines)*30
	for _, l := range lines {
		fmt.Fprintf(&text, `<text x="20" y="%d" font-family="Inter,Arial,sans-serif" font-weight="800" font-size="26" fill="#fff">%s</text>`, y, html.EscapeString(strings.ToUpper(l)))
		y += 30
	}
	label := platform
	if typ != "Game" {
		label = typ + " · " + platform
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 400" width="300" height="400">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="hsl(%d 70%% 45%%)"/><stop offset="1" stop-color="hsl(%d 75%% 18%%)"/></linearGradient>
<linearGradient id="f" x1="0" y1="0" x2="0" y2="1"><stop offset="0.45" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity="0.7"/></linearGradient></defs>
<rect width="300" height="400" fill="url(#g)"/>%s<rect width="300" height="400" fill="url(#f)"/>
<rect x="20" y="20" rx="4" width="%d" height="26" fill="#000" opacity="0.45"/><text x="30" y="38" font-family="Inter,Arial,sans-serif" font-size="13" font-weight="700" fill="#fff">%s</text>
%s</svg>`, hue, h2, shapes.String(), 20+len(label)*8, html.EscapeString(strings.ToUpper(label)), text.String())
}
