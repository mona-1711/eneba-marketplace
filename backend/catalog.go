package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type ProductSummary struct {
	ID          int    `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Platform    string `json:"platform"`
	Device      string `json:"device"`
	Region      string `json:"region"`
	Type        string `json:"type"`
	Genre       string `json:"genre"`
	ReleaseYear int    `json:"releaseYear"`
	CoverURL    string `json:"coverUrl"`
	PriceCents  *int   `json:"priceCents"` // cheapest in-stock offer, null if sold out
	RetailCents int    `json:"retailCents"`
	DiscountPct int    `json:"discountPct"`
	OfferCount  int    `json:"offerCount"`
	Sales       int    `json:"sales"`
}

// listingCTE flattens product + cheapest in-stock offer + retail price into one row per product.
const listingCTE = `
WITH listing AS (
	SELECT p.id, p.slug, p.name, p.title, p.platform, p.device, p.region, p.type, p.genre,
	       p.release_year, p.sales_count, p.created_at,
	       MIN(CASE WHEN o.stock > 0 THEN o.price_cents END) AS min_price,
	       rp.price_cents AS retail,
	       COUNT(CASE WHEN o.stock > 0 THEN 1 END) AS offer_count
	FROM products p
	JOIN retail_prices rp ON rp.product_id = p.id
	LEFT JOIN offers o ON o.product_id = p.id
	GROUP BY p.id
)
`

const listingCols = `id, slug, name, title, platform, device, region, type, genre, release_year, sales_count, min_price, retail, offer_count`

func scanSummaries(rows *sql.Rows) ([]ProductSummary, error) {
	defer rows.Close()
	out := []ProductSummary{}
	for rows.Next() {
		var p ProductSummary
		var minPrice sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.Title, &p.Platform, &p.Device, &p.Region, &p.Type, &p.Genre,
			&p.ReleaseYear, &p.Sales, &minPrice, &p.RetailCents, &p.OfferCount); err != nil {
			return nil, err
		}
		if minPrice.Valid {
			v := int(minPrice.Int64)
			p.PriceCents = &v
			if p.RetailCents > v {
				p.DiscountPct = int(float64(p.RetailCents-v)/float64(p.RetailCents)*100 + 0.5)
			}
		}
		p.CoverURL = "/api/covers/" + p.Slug
		out = append(out, p)
	}
	return out, rows.Err()
}

// GET /api/products?q=&platform=&region=&type=&genre=&minPrice=&maxPrice=&inStock=1&sort=&page=&pageSize=
// platform/region/type/genre accept multiple values (repeat the param).
func (a *App) handleProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where := []string{"1=1"}
	args := []any{}

	addIn := func(col, param string) {
		vals := q[param]
		if len(vals) == 0 {
			return
		}
		ph := make([]string, len(vals))
		for i, v := range vals {
			ph[i] = "?"
			args = append(args, v)
		}
		where = append(where, fmt.Sprintf("%s IN (%s)", col, strings.Join(ph, ",")))
	}
	addIn("platform", "platform")
	addIn("region", "region")
	addIn("type", "type")
	addIn("genre", "genre")
	addIn("device", "device")

	if v, err := strconv.ParseFloat(q.Get("minPrice"), 64); err == nil {
		where = append(where, "min_price >= ?")
		args = append(args, int(v*100))
	}
	if v, err := strconv.ParseFloat(q.Get("maxPrice"), 64); err == nil {
		where = append(where, "min_price <= ?")
		args = append(args, int(v*100))
	}
	if q.Get("inStock") == "1" {
		where = append(where, "min_price IS NOT NULL")
	}

	// Search: the index returns matching IDs with relevance scores.
	query := strings.TrimSpace(q.Get("q"))
	var hits []SearchHit
	relevance := map[int]int{}
	if query != "" {
		hits = a.search.Search(query)
		if len(hits) == 0 {
			writeJSON(w, 200, map[string]any{"items": []ProductSummary{}, "total": 0, "page": 1, "pageSize": 24, "didYouMean": ""})
			return
		}
		ph := make([]string, len(hits))
		for i, h := range hits {
			ph[i] = "?"
			args = append(args, h.ID)
			relevance[h.ID] = i
		}
		where = append(where, fmt.Sprintf("id IN (%s)", strings.Join(ph, ",")))
	}

	sortKey := q.Get("sort")
	if sortKey == "" {
		if query != "" {
			sortKey = "relevance"
		} else {
			sortKey = "popular"
		}
	}
	orderBy := map[string]string{
		"popular":    "sales_count DESC",
		"price_asc":  "min_price IS NULL, min_price ASC",
		"price_desc": "min_price IS NULL, min_price DESC",
		"newest":     "release_year DESC, created_at DESC",
		"discount":   "min_price IS NULL, (retail - min_price) * 1.0 / retail DESC",
		"name":       "name COLLATE NOCASE ASC",
		"relevance":  "sales_count DESC", // re-sorted in Go by search score below
	}[sortKey]
	if orderBy == "" {
		writeErr(w, 400, "unknown sort")
		return
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 60 {
		pageSize = 24
	}

	sqlStr := listingCTE + `SELECT ` + listingCols + ` FROM listing WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ` + orderBy + `, id`
	rows, err := a.db.Query(sqlStr, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	items, err := scanSummaries(rows)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if sortKey == "relevance" {
		sortByRelevance(items, relevance)
	}
	total := len(items)
	start := min((page-1)*pageSize, total)
	end := min(start+pageSize, total)

	writeJSON(w, 200, map[string]any{
		"items":      items[start:end],
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
		"didYouMean": a.search.Suggest(hits),
	})
}

func sortByRelevance(items []ProductSummary, rank map[int]int) {
	// insertion sort keeps it dependency-free and stable; lists are small
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && rank[items[j].ID] < rank[items[j-1].ID]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// GET /api/search/suggest?q= : compact results for the header search dropdown.
func (a *App) handleSuggest(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, 200, map[string]any{"items": []ProductSummary{}, "didYouMean": ""})
		return
	}
	hits := a.search.Search(query)
	if len(hits) > 6 {
		hits = hits[:6]
	}
	items, err := a.summariesByIDs(hits)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "didYouMean": a.search.Suggest(hits)})
}

func (a *App) summariesByIDs(hits []SearchHit) ([]ProductSummary, error) {
	if len(hits) == 0 {
		return []ProductSummary{}, nil
	}
	ph := make([]string, len(hits))
	args := make([]any, len(hits))
	rank := map[int]int{}
	for i, h := range hits {
		ph[i], args[i], rank[h.ID] = "?", h.ID, i
	}
	rows, err := a.db.Query(listingCTE+`SELECT `+listingCols+` FROM listing WHERE id IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	items, err := scanSummaries(rows)
	if err != nil {
		return nil, err
	}
	sortByRelevance(items, rank)
	return items, nil
}

type Offer struct {
	ID            int     `json:"id"`
	SellerID      int     `json:"sellerId"`
	SellerName    string  `json:"sellerName"`
	SellerRating  float64 `json:"sellerRating"`
	SellerReviews int     `json:"sellerReviews"`
	PriceCents    int     `json:"priceCents"`
	Stock         int     `json:"stock"`
}

type ProductDetail struct {
	ProductSummary
	Description string           `json:"description"`
	Developer   string           `json:"developer"`
	Offers      []Offer          `json:"offers"`
	Editions    []ProductSummary `json:"editions"` // same title on other platforms/regions
	Related     []ProductSummary `json:"related"`
}

func (a *App) handleProduct(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	rows, err := a.db.Query(listingCTE+`SELECT `+listingCols+` FROM listing WHERE slug = ?`, slug)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	list, err := scanSummaries(rows)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(list) == 0 {
		writeErr(w, 404, "product not found")
		return
	}
	d := ProductDetail{ProductSummary: list[0]}
	if err := a.db.QueryRow(`SELECT description, developer FROM products WHERE id=?`, d.ID).Scan(&d.Description, &d.Developer); err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	orows, err := a.db.Query(`SELECT o.id, s.id, s.name, s.rating, s.reviews, o.price_cents, o.stock
		FROM offers o JOIN sellers s ON s.id = o.seller_id
		WHERE o.product_id = ? ORDER BY o.stock = 0, o.price_cents ASC`, d.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer orows.Close()
	d.Offers = []Offer{}
	for orows.Next() {
		var o Offer
		if err := orows.Scan(&o.ID, &o.SellerID, &o.SellerName, &o.SellerRating, &o.SellerReviews, &o.PriceCents, &o.Stock); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		d.Offers = append(d.Offers, o)
	}

	if rows, err = a.db.Query(listingCTE+`SELECT `+listingCols+` FROM listing WHERE title = ? AND id != ? ORDER BY platform, region`, d.Title, d.ID); err == nil {
		d.Editions, _ = scanSummaries(rows)
	}
	if rows, err = a.db.Query(listingCTE+`SELECT `+listingCols+` FROM listing WHERE genre = ? AND title != ? AND region = 'GLOBAL' AND min_price IS NOT NULL
		GROUP BY title ORDER BY sales_count DESC LIMIT 8`, d.Genre, d.Title); err == nil {
		d.Related, _ = scanSummaries(rows)
	}
	writeJSON(w, 200, d)
}

type Facet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// GET /api/facets : filter options with counts, so the store sidebar is data-driven too.
func (a *App) handleFacets(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}
	for _, col := range []string{"platform", "region", "type", "genre"} {
		rows, err := a.db.Query(`SELECT ` + col + `, COUNT(*) FROM products GROUP BY 1 ORDER BY 2 DESC, 1`)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		fs := []Facet{}
		for rows.Next() {
			var f Facet
			if err := rows.Scan(&f.Value, &f.Count); err == nil {
				fs = append(fs, f)
			}
		}
		rows.Close()
		out[col] = fs
	}
	var lo, hi sql.NullInt64
	a.db.QueryRow(`SELECT MIN(price_cents), MAX(price_cents) FROM offers WHERE stock > 0`).Scan(&lo, &hi)
	out["priceRange"] = map[string]int64{"minCents": lo.Int64, "maxCents": hi.Int64}
	writeJSON(w, 200, out)
}

type CartLine struct {
	OfferID  int `json:"offerId"`
	Quantity int `json:"quantity"`
}

type QuotedLine struct {
	OfferID     int    `json:"offerId"`
	Quantity    int    `json:"quantity"`
	ProductSlug string `json:"productSlug"`
	ProductName string `json:"productName"`
	CoverURL    string `json:"coverUrl"`
	SellerName  string `json:"sellerName"`
	PriceCents  int    `json:"priceCents"`
	Stock       int    `json:"stock"`
	Available   bool   `json:"available"`
}

// POST /api/cart/quote : the cart lives in the browser (works for guests), but prices
// and stock always come from the server so the client can't be trusted with totals.
func (a *App) handleCartQuote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []CartLine `json:"items"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	lines, total, err := a.quote(a.db, body.Items)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": lines, "totalCents": total})
}

type querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func (a *App) quote(db querier, items []CartLine) ([]QuotedLine, int, error) {
	lines := []QuotedLine{}
	total := 0
	for _, it := range items {
		if it.Quantity < 1 {
			continue
		}
		var l QuotedLine
		err := db.QueryRow(`SELECT o.id, p.slug, p.name, s.name, o.price_cents, o.stock
			FROM offers o JOIN products p ON p.id = o.product_id JOIN sellers s ON s.id = o.seller_id WHERE o.id = ?`, it.OfferID).
			Scan(&l.OfferID, &l.ProductSlug, &l.ProductName, &l.SellerName, &l.PriceCents, &l.Stock)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		l.Quantity = it.Quantity
		l.CoverURL = "/api/covers/" + l.ProductSlug
		l.Available = l.Stock >= it.Quantity
		if l.Available {
			total += l.PriceCents * l.Quantity
		}
		lines = append(lines, l)
	}
	return lines, total, nil
}
