package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestApp(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	db, err := openDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	idx, err := buildIndex(db)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{db: db, search: idx}
	srv := httptest.NewServer(app.routes())
	t.Cleanup(func() { srv.Close(); db.Close() })
	return app, srv
}

func topTitle(t *testing.T, app *App, q string) string {
	t.Helper()
	hits := app.search.Search(q)
	if len(hits) == 0 {
		return ""
	}
	var title string
	app.db.QueryRow(`SELECT title FROM products WHERE id = ?`, hits[0].ID).Scan(&title)
	return title
}

func TestSearchToleratesTyposAndPartials(t *testing.T) {
	app, _ := newTestApp(t)
	cases := map[string]string{
		"elden ring":     "Elden Ring",
		"eldn rnig":      "Elden Ring",
		"eldenring":      "Elden Ring",
		"cyberpnk":       "Cyberpunk 2077",
		"cyber":          "Cyberpunk 2077",
		"witchr":         "The Witcher 3 Wild Hunt Complete Edition",
		"baldurs gate":   "Baldur's Gate 3",
		"baldur's":       "Baldur's Gate 3",
		"gta":            "Grand Theft Auto V",
		"rdr2":           "Red Dead Redemption 2",
		"hollow knigth":  "Hollow Knight", // transposition
		"stardw valey":   "Stardew Valley",
		"minecraft xbox": "Minecraft Java & Bedrock Edition",
		"civilization 7": "Sid Meier's Civilization VII",
	}
	for q, want := range cases {
		if got := topTitle(t, app, q); got != want {
			t.Errorf("search %q: top result %q, want %q", q, got, want)
		}
	}
	if hits := app.search.Search("zzqqxx"); len(hits) != 0 {
		t.Errorf("nonsense query matched %d products", len(hits))
	}
}

func TestDamerauLevenshtein(t *testing.T) {
	for _, c := range []struct {
		a, b string
		d    int
	}{{"ring", "ring", 0}, {"rnig", "ring", 1}, {"eldn", "elden", 1}, {"kitten", "sitting", 3}} {
		if got := damerauLevenshtein(c.a, c.b, 5); got != c.d {
			t.Errorf("dl(%q,%q)=%d want %d", c.a, c.b, got, c.d)
		}
	}
}

func TestChargeCard(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	ok := PaymentInput{CardNumber: "4242 4242 4242 4242", Expiry: "12/28", CVC: "123"}
	if _, err := chargeCard(ok, 100, now); err != nil {
		t.Fatalf("test card should pass: %v", err)
	}
	bad := []PaymentInput{
		{CardNumber: "4242 4242 4242 4241", Expiry: "12/28", CVC: "123"}, // Luhn
		{CardNumber: "4000 0000 0000 0002", Expiry: "12/28", CVC: "123"}, // declined
		{CardNumber: "4242424242424242", Expiry: "10/26", CVC: "123"},    // expired
		{CardNumber: "4242424242424242", Expiry: "12/28", CVC: "1"},      // cvc
	}
	for _, p := range bad {
		if _, err := chargeCard(p, 100, now); err == nil {
			t.Errorf("expected failure for %+v", p)
		}
	}
	// expiry month is valid through its last day
	if _, err := chargeCard(PaymentInput{CardNumber: "4242424242424242", Expiry: "11/26", CVC: "123"}, 100, now); err != nil {
		t.Errorf("card expiring this month should pass: %v", err)
	}
}

type client struct {
	t   *testing.T
	srv *httptest.Server
	c   *http.Client
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t, srv, &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, c.srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func TestCheckoutFlow(t *testing.T) {
	app, srv := newTestApp(t)
	c := newClient(t, srv)

	// pick an in-stock offer
	var offerID, stock int
	app.db.QueryRow(`SELECT id, stock FROM offers WHERE stock >= 3 ORDER BY id LIMIT 1`).Scan(&offerID, &stock)
	cart := []CartLine{{OfferID: offerID, Quantity: 2}}
	pay := PaymentInput{CardNumber: "4242424242424242", Expiry: "12/30", CVC: "123", Holder: "Test"}

	// checkout requires auth
	if s := c.do("POST", "/api/checkout", map[string]any{"items": cart, "payment": pay}, nil); s != 401 {
		t.Fatalf("anonymous checkout: status %d, want 401", s)
	}
	if s := c.do("GET", "/api/orders", nil, nil); s != 401 {
		t.Fatalf("anonymous orders: status %d, want 401", s)
	}

	if s := c.do("POST", "/api/auth/register", map[string]string{"email": "a@b.co", "password": "short"}, nil); s != 400 {
		t.Fatalf("short password: %d", s)
	}
	if s := c.do("POST", "/api/auth/register", map[string]string{"email": "a@b.co", "password": "password123", "name": "A"}, nil); s != 201 {
		t.Fatalf("register: %d", s)
	}

	// declined card leaves stock untouched
	declined := pay
	declined.CardNumber = "4000000000000002"
	if s := c.do("POST", "/api/checkout", map[string]any{"items": cart, "payment": declined}, nil); s != 402 {
		t.Fatalf("declined: %d", s)
	}
	var after int
	app.db.QueryRow(`SELECT stock FROM offers WHERE id = ?`, offerID).Scan(&after)
	if after != stock {
		t.Fatalf("stock changed after declined payment: %d -> %d", stock, after)
	}

	var order Order
	body := map[string]any{"items": cart, "payment": pay, "idempotencyKey": "k1"}
	if s := c.do("POST", "/api/checkout", body, &order); s != 201 {
		t.Fatalf("checkout: %d", s)
	}
	if len(order.Items) != 2 || order.Items[0].KeyCode == order.Items[1].KeyCode || len(order.Items[0].KeyCode) != 17 {
		t.Fatalf("expected 2 distinct keys, got %+v", order.Items)
	}
	app.db.QueryRow(`SELECT stock FROM offers WHERE id = ?`, offerID).Scan(&after)
	if after != stock-2 {
		t.Fatalf("stock %d, want %d", after, stock-2)
	}

	// same idempotency key -> same order, no double charge
	var again Order
	c.do("POST", "/api/checkout", body, &again)
	if again.ID != order.ID {
		t.Fatalf("retry created a new order %d (first %d)", again.ID, order.ID)
	}

	// over-buying fails cleanly
	app.db.Exec(`UPDATE offers SET stock = 1 WHERE id = ?`, offerID)
	if s := c.do("POST", "/api/checkout", map[string]any{"items": []CartLine{{OfferID: offerID, Quantity: 2}}, "payment": pay}, nil); s != 409 {
		t.Fatalf("overbuy: %d", s)
	}

	var list struct{ Items []Order }
	c.do("GET", "/api/orders", nil, &list)
	if len(list.Items) != 1 {
		t.Fatalf("order history has %d orders", len(list.Items))
	}

	// another user can't see it
	other := newClient(t, srv)
	other.do("POST", "/api/auth/register", map[string]string{"email": "x@y.co", "password": "password123"}, nil)
	if s := other.do("GET", "/api/orders/"+itoa(order.ID), nil, nil); s != 404 {
		t.Fatalf("other user's order: %d", s)
	}
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func TestStoreFiltersAndHome(t *testing.T) {
	_, srv := newTestApp(t)
	c := newClient(t, srv)
	var res struct {
		Items []ProductSummary
		Total int
	}
	c.do("GET", "/api/products?platform=Steam&maxPrice=20&sort=price_asc&pageSize=60", nil, &res)
	if res.Total == 0 {
		t.Fatal("no cheap steam products")
	}
	for i, p := range res.Items {
		if p.Platform != "Steam" || p.PriceCents == nil || *p.PriceCents > 2000 {
			t.Fatalf("filter leak: %+v", p)
		}
		if i > 0 && *res.Items[i-1].PriceCents > *p.PriceCents {
			t.Fatal("not sorted by price")
		}
	}
	c.do("GET", "/api/products?q=eldn+ring", nil, &res)
	if res.Total == 0 || !strings.HasPrefix(res.Items[0].Name, "Elden Ring") {
		t.Fatalf("store search: %+v", res.Items)
	}

	var home struct{ Blocks []HomeBlock }
	c.do("GET", "/api/home", nil, &home)
	if len(home.Blocks) < 4 {
		t.Fatalf("home blocks: %d", len(home.Blocks))
	}
	for _, b := range home.Blocks {
		var block struct{ Items []json.RawMessage }
		if s := c.do("GET", b.Source, nil, &block); s != 200 || len(block.Items) == 0 {
			t.Errorf("block %s (%s): status %d, %d items", b.Title, b.Source, s, len(block.Items))
		}
	}
}
