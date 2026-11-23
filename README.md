# keyport: an Eneba-style game key marketplace

A marketplace for digital game keys, gift cards and subscriptions, built for the Eneba Intern Assignment 2027. It's a React + TypeScript frontend on a Go REST API with SQLite, shipped as one Docker image.

- **Live demo:** https://keyport-marketplace.onrender.com (Render free tier: the first request after ~15 min idle takes up to a minute to wake the service)
- **Agent chat history:** [`docs/agent-chat-log.md`](docs/agent-chat-log.md)

**Test card:** `4242 4242 4242 4242`, any future expiry, any CVC. Use `4000 0000 0000 0002` to see a declined card.

## What's in it

| Part of the brief | How it's covered |
|---|---|
| **Store** | `/store` lets you filter by platform, region, type, genre, price range and in-stock, using multi-select checkboxes with live counts from `/api/facets`. There are 7 sort orders, search, and pagination. All state lives in the URL, so a filtered view can be shared and the back button works. |
| **Homepage from blocks** | `GET /api/home` reads the `home_blocks` table and returns an ordered list of `{kind, title, source}`. Each block then fetches its own endpoint: banners come from the `banners` table, categories from the `categories` table, and best sellers, deals, new releases and under-€10 are separate catalog queries. The frontend only maps `kind` to a component. Reordering, adding or turning off a block is a DB change with no frontend deploy, and an unknown `kind` is skipped safely. |
| **Typo-tolerant search** | The search box in the header is on every page and shows typeahead results; Enter opens the full results in the store. The backend matches in tiers: exact, then prefix, then substring, then Damerau-Levenshtein (1 typo allowed for words up to 5 letters, 2 for longer ones), plus a prefix-with-typo check. It also handles glued words (`eldenring`), acronyms (`gta`, `rdr2`) and roman numerals (`civilization 7`). A query that looks misspelled shows "Showing matches for …". This is covered by `TestSearchToleratesTyposAndPartials`. |
| **Product page** | Shows details, region and platform badges, and every seller's offer with price, stock, rating and review count. The best price is highlighted and sold-out offers are greyed out. It also links to other editions of the same game (other platforms and regions) and to related games. |
| **Checkout + key delivery** | The cart is stored in localStorage, so guests can shop. The server always re-prices it (`/api/cart/quote`) and never trusts prices from the client. Checkout runs as **one DB transaction**: re-price, reserve stock with a conditional `UPDATE … WHERE stock >= qty` so two buyers can't get the last key, charge the simulated card (Luhn, expiry and CVC checks, plus decline test cards), then issue one unique key per unit. A failed payment rolls back the stock. An **idempotency key** means a double-clicked "Pay" can't charge twice. Keys appear right after payment and stay in order history behind a "Reveal key" button. |
| **Auth** | Register and sign in with bcrypt-hashed passwords. Sessions use an opaque random token in an `HttpOnly`, `SameSite=Lax` cookie, stored server-side so logout really revokes it. Checkout and orders are protected on the server (`401`) and on the client (redirect to sign-in, then back to where you were). Users can't read each other's orders (`404`). |
| **Eneba styling** | Deep purple surfaces, a purple sticky header with a white search bar, yellow CTAs, green discount badges and region labels, and a bold geometric sans font (Montserrat, standing in for Eneba's Metropolis). The layout uses dense 3:4 cover cards with "From €x" pricing and seller offer rows. It's responsive down to phone width, where filters open as a full-screen sheet. |

## Scope decisions (and why)

- **One deployable, not two.** Go serves the built SPA and the API from the same origin. That means one free Render service and no CORS, and the session cookie stays first-party with no third-party-cookie problems. The frontend still supports `VITE_API_URL`, and the backend supports `CORS_ORIGINS`, if you want to put it on Vercel separately.
- **SQLite instead of Postgres.** The data is small and relational, and SQLite needs zero setup. It's the pure-Go driver (`modernc.org/sqlite`), so the binary is static and has no CGO. **Trade-off:** Render's free disk is ephemeral, so accounts and orders reset when the service restarts. The catalog re-seeds deterministically. Switching to Postgres would mean changing the driver and a few `datetime('now')` calls.
- **In-memory search, not Elasticsearch/Meilisearch.** About 170 listings fit in memory easily, and every query is under 1 ms. A dedicated engine would make sense at real catalog sizes. The scoring here (tiered match plus edit distance plus title coverage) is the same idea, just explicit and testable.
- **Keys are generated at purchase.** Each offer has a stock count, and a unique key in the format `XXXXX-XXXXX-XXXXX` (an alphabet with no ambiguous characters) is created inside the checkout transaction. A real marketplace would hold seller-uploaded key inventory. The order and stock logic would be the same.
- **Simulated payments instead of Stripe test mode.** This needs no account and no secrets to deploy. The simulator behaves like a gateway: validation, decline codes, a payment reference, and only the last 4 digits stored.
- **Generated cover art.** I didn't hotlink or ship publisher artwork, for licensing reasons. `/api/covers/{slug}` draws a deterministic SVG cover for each listing. Game titles are used only as product names.
- **Own brand name.** The brief asks for Eneba's look and feel, so colors, type and components follow it. I used a different wordmark (keyport) instead of copying Eneba's logo, so a public demo can't be mistaken for the real store.
- **Not built, on purpose:** seller dashboards, reviews, wishlists, real email delivery, and admin UI for the homepage blocks (it's DB-driven, but you edit it with SQL). The brief says "a few features done extensively beat many minimal ones", so the effort went into search, checkout correctness and the block system.

## Architecture

```
frontend/  React 18 + TS + Vite + react-router
  src/lib/        api client (typed), auth context, cart context (localStorage), useFetch
  src/components/ Layout/header, SearchBox (typeahead), ProductCard, Blocks (block renderers)
  src/pages/      Home, Store, Product, Cart, Checkout, Orders, Auth
backend/   Go 1.26, net/http (method+path routing), SQLite
  main.go      routes, SPA serving, CORS, logging
  db.go        schema + deterministic seed (62 games -> ~170 listings, 8 sellers, 2-6 offers each)
  search.go    typo-tolerant search index
  catalog.go   products, filters, facets, product detail, cart quote
  blocks.go    homepage composition + one handler per block, SVG covers
  auth.go      register/login/logout/me, session middleware
  checkout.go  payment simulator, transactional checkout, orders
  app_test.go  search, payment, end-to-end checkout/auth, filters, homepage blocks
```

### Data model

`products` (one row per listing: title × platform × region), `sellers`, `offers` (product × seller, price and stock), `retail_prices`, `users`, `sessions`, `orders`, `order_items` (holds the delivered key), `home_blocks`, `banners`, `categories`.

### API

```
GET  /api/home                      ordered homepage blocks
GET  /api/blocks/{banners|categories|bestsellers|deals|new-releases|budget}
GET  /api/products?q&platform&region&type&genre&minPrice&maxPrice&inStock&sort&page
GET  /api/products/{slug}           detail + offers + editions + related
GET  /api/facets                    filter options with counts
GET  /api/search/suggest?q          typeahead
POST /api/cart/quote                server-side pricing of a cart
POST /api/auth/{register|login|logout}   GET /api/auth/me
POST /api/checkout                  (auth) pay + receive keys
GET  /api/orders, /api/orders/{id}  (auth)
```

## Run locally

```bash
# backend (seeds marketplace.db on first run)
cd backend && go run .            # :8080

# frontend (proxies /api to :8080)
cd frontend && npm install && npm run dev   # :5173

# tests
cd backend && go test ./...

# or everything in one container
docker build -t keyport . && docker run -p 8080:8080 keyport
```

## Deploy (Render, free tier)

The repo includes a `render.yaml` blueprint. In Render, choose **New → Blueprint**, pick this repo, and click **Apply**. Render builds the Dockerfile and serves the app on one URL.

## How I used AI

This was built with Claude Code as an agent, working from the assignment PDF. I set the scope and defaults (stack, deploy target, a single service), and the agent wrote the code, tests and this README. It also ran a headless browser to click through the full flow (search → product → cart → register → declined card → successful payment → key reveal), which caught a real bug: a `useEffect` that returned a value and crashed client-side navigation. The full conversation is in [`docs/agent-chat-log.md`](docs/agent-chat-log.md).
