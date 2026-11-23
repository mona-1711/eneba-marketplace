package main

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"strings"

	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS products (
	id            INTEGER PRIMARY KEY,
	slug          TEXT NOT NULL UNIQUE,
	title         TEXT NOT NULL,   -- base game title, e.g. "Elden Ring"
	name          TEXT NOT NULL,   -- listing name, e.g. "Elden Ring (PC) Steam Key GLOBAL"
	platform      TEXT NOT NULL,   -- Steam, Xbox, PSN, Nintendo, EA App, Ubisoft Connect, GOG
	device        TEXT NOT NULL,   -- PC, Xbox Series X|S, PS5, Switch ...
	region        TEXT NOT NULL,   -- GLOBAL, EUROPE, NORTH AMERICA ...
	type          TEXT NOT NULL,   -- Game, DLC, Gift Card, Subscription
	genre         TEXT NOT NULL,
	developer     TEXT NOT NULL,
	release_year  INTEGER NOT NULL,
	description   TEXT NOT NULL,
	cover_hue     INTEGER NOT NULL, -- used by the generated SVG cover
	sales_count   INTEGER NOT NULL DEFAULT 0,
	created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sellers (
	id         INTEGER PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE,
	rating     REAL NOT NULL,     -- 0..100 positive feedback %
	reviews    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS offers (
	id              INTEGER PRIMARY KEY,
	product_id      INTEGER NOT NULL REFERENCES products(id),
	seller_id       INTEGER NOT NULL REFERENCES sellers(id),
	price_cents     INTEGER NOT NULL,
	stock           INTEGER NOT NULL,
	UNIQUE(product_id, seller_id)
);
CREATE INDEX IF NOT EXISTS idx_offers_product ON offers(product_id);

-- Retail (MSRP) price per product, used to compute the "-XX%" discount badge.
CREATE TABLE IF NOT EXISTS retail_prices (
	product_id   INTEGER PRIMARY KEY REFERENCES products(id),
	price_cents  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY,
	email         TEXT NOT NULL UNIQUE,
	name          TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id),
	expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS orders (
	id           INTEGER PRIMARY KEY,
	user_id      INTEGER NOT NULL REFERENCES users(id),
	total_cents  INTEGER NOT NULL,
	status       TEXT NOT NULL,
	payment_ref  TEXT NOT NULL,
	card_last4   TEXT NOT NULL,
	idempotency_key TEXT UNIQUE, -- client-generated, makes double-clicking "Pay" safe
	created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS order_items (
	id           INTEGER PRIMARY KEY,
	order_id     INTEGER NOT NULL REFERENCES orders(id),
	offer_id     INTEGER NOT NULL REFERENCES offers(id),
	product_id   INTEGER NOT NULL REFERENCES products(id),
	seller_name  TEXT NOT NULL,
	product_name TEXT NOT NULL,
	price_cents  INTEGER NOT NULL,
	key_code     TEXT NOT NULL
);

-- Homepage layout: which blocks appear, in what order. The frontend renders
-- whatever this table says; each block's content comes from its own endpoint.
CREATE TABLE IF NOT EXISTS home_blocks (
	id        INTEGER PRIMARY KEY,
	position  INTEGER NOT NULL,
	kind      TEXT NOT NULL,     -- banner | product_rail | categories
	title     TEXT NOT NULL,
	source    TEXT NOT NULL,     -- API path the frontend fetches for content
	enabled   INTEGER NOT NULL DEFAULT 1
);

-- Banner block has its own content table.
CREATE TABLE IF NOT EXISTS banners (
	id          INTEGER PRIMARY KEY,
	position    INTEGER NOT NULL,
	headline    TEXT NOT NULL,
	subline     TEXT NOT NULL,
	cta_label   TEXT NOT NULL,
	cta_link    TEXT NOT NULL,
	product_id  INTEGER REFERENCES products(id),
	hue         INTEGER NOT NULL
);

-- Category tiles block has its own content table.
CREATE TABLE IF NOT EXISTS categories (
	id        INTEGER PRIMARY KEY,
	position  INTEGER NOT NULL,
	label     TEXT NOT NULL,
	link      TEXT NOT NULL,
	hue       INTEGER NOT NULL
);
`

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: serialize writes, keeps checkout transactions simple and safe
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		if err := seed(db); err != nil {
			return nil, fmt.Errorf("seed: %w", err)
		}
	}
	return db, nil
}

type seedGame struct {
	title, genre, dev string
	year              int
	baseEUR           float64
	platforms         []string // subset of platformDefs keys
	kind              string
}

type platformDef struct{ platform, device string }

var platformDefs = map[string]platformDef{
	"steam":    {"Steam", "PC"},
	"xbox":     {"Xbox Live", "Xbox Series X|S"},
	"psn":      {"PSN", "PS5"},
	"nintendo": {"Nintendo eShop", "Nintendo Switch"},
	"ea":       {"EA App", "PC"},
	"ubisoft":  {"Ubisoft Connect", "PC"},
	"gog":      {"GOG", "PC"},
}

var seedGames = []seedGame{
	{"Elden Ring", "RPG", "FromSoftware", 2022, 59.99, []string{"steam", "xbox", "psn"}, "Game"},
	{"Elden Ring Shadow of the Erdtree", "RPG", "FromSoftware", 2024, 39.99, []string{"steam", "psn"}, "DLC"},
	{"Cyberpunk 2077", "RPG", "CD PROJEKT RED", 2020, 59.99, []string{"steam", "gog", "xbox"}, "Game"},
	{"Cyberpunk 2077 Phantom Liberty", "RPG", "CD PROJEKT RED", 2023, 29.99, []string{"steam", "gog"}, "DLC"},
	{"The Witcher 3 Wild Hunt Complete Edition", "RPG", "CD PROJEKT RED", 2016, 49.99, []string{"steam", "gog", "nintendo"}, "Game"},
	{"Baldur's Gate 3", "RPG", "Larian Studios", 2023, 59.99, []string{"steam", "gog", "psn"}, "Game"},
	{"Red Dead Redemption 2", "Action", "Rockstar Games", 2019, 59.99, []string{"steam", "xbox"}, "Game"},
	{"Grand Theft Auto V", "Action", "Rockstar Games", 2015, 29.99, []string{"steam", "xbox", "psn"}, "Game"},
	{"Hogwarts Legacy", "Adventure", "Avalanche Software", 2023, 59.99, []string{"steam", "xbox", "psn"}, "Game"},
	{"Minecraft Java & Bedrock Edition", "Sandbox", "Mojang Studios", 2011, 29.99, []string{"xbox"}, "Game"},
	{"Stardew Valley", "Simulation", "ConcernedApe", 2016, 14.99, []string{"steam", "gog", "nintendo"}, "Game"},
	{"Hades II", "Roguelike", "Supergiant Games", 2025, 29.99, []string{"steam", "nintendo"}, "Game"},
	{"Hollow Knight Silksong", "Platformer", "Team Cherry", 2025, 19.99, []string{"steam", "nintendo", "xbox"}, "Game"},
	{"Starfield", "RPG", "Bethesda Game Studios", 2023, 69.99, []string{"steam", "xbox"}, "Game"},
	{"Forza Horizon 5", "Racing", "Playground Games", 2021, 59.99, []string{"steam", "xbox"}, "Game"},
	{"EA SPORTS FC 26", "Sports", "EA Vancouver", 2025, 69.99, []string{"ea", "psn", "xbox"}, "Game"},
	{"Battlefield 6", "Shooter", "Battlefield Studios", 2025, 69.99, []string{"steam", "ea", "psn"}, "Game"},
	{"Call of Duty Black Ops 7", "Shooter", "Treyarch", 2025, 69.99, []string{"steam", "xbox", "psn"}, "Game"},
	{"Assassin's Creed Shadows", "Action", "Ubisoft Quebec", 2025, 69.99, []string{"ubisoft", "xbox", "psn"}, "Game"},
	{"Far Cry 6", "Shooter", "Ubisoft Toronto", 2021, 59.99, []string{"ubisoft", "xbox"}, "Game"},
	{"Monster Hunter Wilds", "Action", "Capcom", 2025, 69.99, []string{"steam", "psn", "xbox"}, "Game"},
	{"Resident Evil 4", "Horror", "Capcom", 2023, 39.99, []string{"steam", "psn"}, "Game"},
	{"Silent Hill 2", "Horror", "Bloober Team", 2024, 69.99, []string{"steam", "psn"}, "Game"},
	{"Sekiro Shadows Die Twice", "Action", "FromSoftware", 2019, 59.99, []string{"steam", "xbox"}, "Game"},
	{"Dark Souls III", "RPG", "FromSoftware", 2016, 59.99, []string{"steam", "xbox"}, "Game"},
	{"Terraria", "Sandbox", "Re-Logic", 2011, 9.99, []string{"steam", "gog", "nintendo"}, "Game"},
	{"Hollow Knight", "Platformer", "Team Cherry", 2017, 14.99, []string{"steam", "gog", "nintendo"}, "Game"},
	{"Celeste", "Platformer", "Maddy Makes Games", 2018, 19.99, []string{"steam", "nintendo"}, "Game"},
	{"It Takes Two", "Adventure", "Hazelight Studios", 2021, 39.99, []string{"steam", "ea", "psn"}, "Game"},
	{"Split Fiction", "Adventure", "Hazelight Studios", 2025, 49.99, []string{"steam", "ea", "psn"}, "Game"},
	{"Palworld", "Survival", "Pocketpair", 2024, 29.99, []string{"steam", "xbox"}, "Game"},
	{"Valheim", "Survival", "Iron Gate", 2021, 19.99, []string{"steam", "xbox"}, "Game"},
	{"Sons of the Forest", "Survival", "Endnight Games", 2023, 29.99, []string{"steam"}, "Game"},
	{"Satisfactory", "Simulation", "Coffee Stain Studios", 2024, 39.99, []string{"steam"}, "Game"},
	{"Cities Skylines II", "Simulation", "Colossal Order", 2023, 49.99, []string{"steam", "xbox"}, "Game"},
	{"Euro Truck Simulator 2", "Simulation", "SCS Software", 2012, 19.99, []string{"steam"}, "Game"},
	{"Microsoft Flight Simulator 2024", "Simulation", "Asobo Studio", 2024, 69.99, []string{"steam", "xbox"}, "Game"},
	{"Sid Meier's Civilization VII", "Strategy", "Firaxis Games", 2025, 69.99, []string{"steam", "nintendo"}, "Game"},
	{"Total War WARHAMMER III", "Strategy", "Creative Assembly", 2022, 59.99, []string{"steam"}, "Game"},
	{"Age of Empires IV", "Strategy", "Relic Entertainment", 2021, 39.99, []string{"steam", "xbox"}, "Game"},
	{"Doom The Dark Ages", "Shooter", "id Software", 2025, 69.99, []string{"steam", "xbox", "psn"}, "Game"},
	{"Helldivers 2", "Shooter", "Arrowhead Game Studios", 2024, 39.99, []string{"steam", "psn"}, "Game"},
	{"Ghost of Tsushima Director's Cut", "Action", "Sucker Punch Productions", 2024, 59.99, []string{"steam", "psn"}, "Game"},
	{"God of War Ragnarök", "Action", "Santa Monica Studio", 2024, 59.99, []string{"steam", "psn"}, "Game"},
	{"Marvel's Spider-Man 2", "Action", "Insomniac Games", 2025, 59.99, []string{"steam", "psn"}, "Game"},
	{"The Legend of Zelda Tears of the Kingdom", "Adventure", "Nintendo", 2023, 69.99, []string{"nintendo"}, "Game"},
	{"Mario Kart 8 Deluxe", "Racing", "Nintendo", 2017, 59.99, []string{"nintendo"}, "Game"},
	{"Super Smash Bros Ultimate", "Fighting", "Bandai Namco", 2018, 59.99, []string{"nintendo"}, "Game"},
	{"Tekken 8", "Fighting", "Bandai Namco", 2024, 69.99, []string{"steam", "psn", "xbox"}, "Game"},
	{"Street Fighter 6", "Fighting", "Capcom", 2023, 59.99, []string{"steam", "psn", "xbox"}, "Game"},
	{"Lies of P", "RPG", "Neowiz", 2023, 59.99, []string{"steam", "xbox"}, "Game"},
	{"Clair Obscur Expedition 33", "RPG", "Sandfall Interactive", 2025, 49.99, []string{"steam", "xbox", "psn"}, "Game"},
	{"Kingdom Come Deliverance II", "RPG", "Warhorse Studios", 2025, 59.99, []string{"steam", "xbox", "psn"}, "Game"},
	{"Disco Elysium The Final Cut", "RPG", "ZA/UM", 2021, 39.99, []string{"steam", "gog"}, "Game"},
	{"The Sims 4 Cottage Living", "Simulation", "Maxis", 2021, 39.99, []string{"ea"}, "DLC"},
	{"Rainbow Six Siege", "Shooter", "Ubisoft Montreal", 2015, 19.99, []string{"ubisoft", "xbox"}, "Game"},
	{"Steam Gift Card 20 EUR", "Gift Card", "Valve", 2024, 20.00, []string{"steam"}, "Gift Card"},
	{"Steam Gift Card 50 EUR", "Gift Card", "Valve", 2024, 50.00, []string{"steam"}, "Gift Card"},
	{"PlayStation Network Card 25 EUR", "Gift Card", "Sony", 2024, 25.00, []string{"psn"}, "Gift Card"},
	{"Xbox Game Pass Ultimate 3 Months", "Subscription", "Microsoft", 2024, 59.99, []string{"xbox"}, "Subscription"},
	{"PlayStation Plus Essential 12 Months", "Subscription", "Sony", 2024, 79.99, []string{"psn"}, "Subscription"},
	{"Nintendo Switch Online 12 Months", "Subscription", "Nintendo", 2024, 19.99, []string{"nintendo"}, "Subscription"},
}

var regions = []string{"GLOBAL", "EUROPE", "NORTH AMERICA", "UNITED KINGDOM"}

var sellerSeed = []struct {
	name    string
	rating  float64
	reviews int
}{
	{"PixelVault", 99.2, 184320},
	{"KeyKingdom", 98.7, 92110},
	{"GameDropz", 97.9, 45012},
	{"LootLlama", 99.6, 310455},
	{"NordicKeys", 96.4, 12044},
	{"BudgetBytes", 94.8, 8120},
	{"RapidCodes", 98.1, 66300},
	{"ArcadeAtlas", 97.2, 20777},
}

func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == '\'' || r == 'ö':
			if r == 'ö' {
				b.WriteRune('o')
			}
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func listingName(g seedGame, p platformDef, region string) string {
	switch g.kind {
	case "Gift Card", "Subscription":
		return fmt.Sprintf("%s %s Key %s", g.title, p.platform, region)
	}
	label := "Key"
	if p.platform == "Xbox Live" || p.platform == "PSN" || p.platform == "Nintendo eShop" {
		label = "Key"
	}
	return fmt.Sprintf("%s (%s) %s %s %s", g.title, p.device, p.platform, label, region)
}

func describe(g seedGame, p platformDef, region string) string {
	switch g.kind {
	case "Gift Card":
		return fmt.Sprintf("Top up your %s wallet. This digital code is delivered instantly after payment and can be redeemed on accounts registered in the %s region.", p.platform, strings.ToLower(region))
	case "Subscription":
		return fmt.Sprintf("%s, delivered as a digital code. Redeem it on your %s account to activate or extend your membership.", g.title, p.platform)
	}
	what := "a game"
	if g.kind == "DLC" {
		what = "an expansion (requires the base game)"
	}
	return fmt.Sprintf("%s is %s in the %s genre from %s, released in %d. This is a digital %s key for %s, activatable in the %s region. You receive the key instantly after payment and redeem it on %s.",
		g.title, what, strings.ToLower(g.genre), g.dev, g.year, p.platform, p.device, strings.ToLower(region), p.platform)
}

// seed fills the catalog with deterministic demo data (rand seeded with a constant).
func seed(db *sql.DB) error {
	r := rand.New(rand.NewSource(42))
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, s := range sellerSeed {
		if _, err := tx.Exec(`INSERT INTO sellers(id,name,rating,reviews) VALUES(?,?,?,?)`, i+1, s.name, s.rating, s.reviews); err != nil {
			return err
		}
	}

	productID := 0
	firstProduct := map[string]int{}
	for gi, g := range seedGames {
		for pi, pk := range g.platforms {
			p := platformDefs[pk]
			// Most listings are GLOBAL; some get an extra regional listing (cheaper) like on real key marketplaces.
			regs := []string{"GLOBAL"}
			if g.kind == "Gift Card" {
				regs = []string{"EUROPE"}
			} else if (gi+pi)%3 == 0 {
				regs = append(regs, regions[1+(gi+pi)%3])
			}
			for ri, region := range regs {
				productID++
				name := listingName(g, p, region)
				slug := slugify(name)
				hue := (gi*47 + pi*23) % 360
				sales := r.Intn(4000) + 50
				if g.year >= 2024 {
					sales += 1500
				}
				_, err := tx.Exec(`INSERT INTO products(id,slug,title,name,platform,device,region,type,genre,developer,release_year,description,cover_hue,sales_count,created_at)
					VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,datetime('now', ?))`,
					productID, slug, g.title, name, p.platform, p.device, region, g.kind, g.genre, g.dev, g.year,
					describe(g, p, region), hue, sales, fmt.Sprintf("-%d days", (len(seedGames)-gi)*3+ri))
				if err != nil {
					return fmt.Errorf("product %s: %w", name, err)
				}
				if _, ok := firstProduct[g.title]; !ok {
					firstProduct[g.title] = productID
				}

				retail := int(math.Round(g.baseEUR * 100))
				if _, err := tx.Exec(`INSERT INTO retail_prices(product_id, price_cents) VALUES(?,?)`, productID, retail); err != nil {
					return err
				}

				// 2..6 sellers per listing, each with its own price and stock.
				perm := r.Perm(len(sellerSeed))
				n := 2 + r.Intn(5)
				ageFactor := 1.0 - float64(2026-g.year)*0.07
				if ageFactor < 0.35 {
					ageFactor = 0.35
				}
				if g.kind == "Gift Card" {
					ageFactor = 0.97
				}
				regionFactor := 1.0
				if region != "GLOBAL" {
					regionFactor = 0.88
				}
				for k := 0; k < n; k++ {
					price := float64(retail) * ageFactor * regionFactor * (0.78 + r.Float64()*0.3)
					if price > float64(retail)*1.02 {
						price = float64(retail) * 1.02
					}
					cents := int(price/10)*10 + 9 // prices end in 9
					stock := r.Intn(60) + 1
					if r.Intn(10) == 0 {
						stock = 0 // a few sold-out offers to make the UI honest
					}
					if _, err := tx.Exec(`INSERT INTO offers(product_id,seller_id,price_cents,stock) VALUES(?,?,?,?)`,
						productID, perm[k]+1, cents, stock); err != nil {
						return err
					}
				}
			}
		}
	}

	banners := []struct {
		head, sub, cta, title string
		hue                   int
	}{
		{"Clair Obscur: Expedition 33", "The breakout RPG of the year, now from multiple sellers at up to -40%.", "Shop now", "Clair Obscur Expedition 33", 28},
		{"Hollow Knight Silksong", "Finally here. Grab your key and get delivered instantly.", "Get it now", "Hollow Knight Silksong", 350},
		{"Autumn Sale on Strategy", "Build empires for less. Civilization VII, Total War and more.", "Browse strategy", "", 200},
	}
	for i, b := range banners {
		var pid any
		link := "/store?genre=Strategy&sort=discount"
		if b.title != "" {
			pid = firstProduct[b.title]
			var slug string
			if err := tx.QueryRow(`SELECT slug FROM products WHERE id=?`, pid).Scan(&slug); err != nil {
				return err
			}
			link = "/product/" + slug
		}
		if _, err := tx.Exec(`INSERT INTO banners(position,headline,subline,cta_label,cta_link,product_id,hue) VALUES(?,?,?,?,?,?,?)`,
			i, b.head, b.sub, b.cta, link, pid, b.hue); err != nil {
			return err
		}
	}

	cats := []struct {
		label, link string
		hue         int
	}{
		{"Steam", "/store?platform=Steam", 210},
		{"Xbox", "/store?platform=Xbox+Live", 120},
		{"PlayStation", "/store?platform=PSN", 230},
		{"Nintendo", "/store?platform=Nintendo+eShop", 0},
		{"Gift Cards", "/store?type=Gift+Card", 45},
		{"Subscriptions", "/store?type=Subscription", 280},
		{"RPG", "/store?genre=RPG", 300},
		{"Shooters", "/store?genre=Shooter", 15},
	}
	for i, c := range cats {
		if _, err := tx.Exec(`INSERT INTO categories(position,label,link,hue) VALUES(?,?,?,?)`, i, c.label, c.link, c.hue); err != nil {
			return err
		}
	}

	blocks := []struct{ kind, title, source string }{
		{"banner", "Featured", "/api/blocks/banners"},
		{"product_rail", "Best sellers", "/api/blocks/bestsellers"},
		{"categories", "Shop by category", "/api/blocks/categories"},
		{"product_rail", "Top deals", "/api/blocks/deals"},
		{"product_rail", "New releases", "/api/blocks/new-releases"},
		{"product_rail", "Under €10", "/api/blocks/budget"},
	}
	for i, b := range blocks {
		if _, err := tx.Exec(`INSERT INTO home_blocks(position,kind,title,source) VALUES(?,?,?,?)`, i, b.kind, b.title, b.source); err != nil {
			return err
		}
	}

	return tx.Commit()
}
