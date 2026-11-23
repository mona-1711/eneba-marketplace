package main

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Simulated payment provider. No real money and no third party: card details are
// validated like a real gateway would (Luhn, expiry, CVC) and a few well-known test
// numbers trigger failures so the error paths can be demoed.
//
//   4242 4242 4242 4242  -> approved
//   4000 0000 0000 0002  -> card declined
//   4000 0000 0000 9995  -> insufficient funds
//   any other Luhn-valid number -> approved
//
// The full card number is never stored; only the last 4 digits.

type PaymentInput struct {
	CardNumber string `json:"cardNumber"`
	Expiry     string `json:"expiry"` // MM/YY
	CVC        string `json:"cvc"`
	Holder     string `json:"holder"`
}

var errPayment = errors.New("payment")

func luhnValid(num string) bool {
	if len(num) < 12 || len(num) > 19 {
		return false
	}
	sum, alt := 0, false
	for i := len(num) - 1; i >= 0; i-- {
		d := int(num[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

// chargeCard returns a payment reference or a user-facing error.
func chargeCard(p PaymentInput, amountCents int, now time.Time) (string, error) {
	num := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, p.CardNumber)
	if !luhnValid(num) {
		return "", fmt.Errorf("%w: That card number isn't valid.", errPayment)
	}
	mm, yy, ok := strings.Cut(strings.TrimSpace(p.Expiry), "/")
	m, err1 := strconv.Atoi(strings.TrimSpace(mm))
	y, err2 := strconv.Atoi(strings.TrimSpace(yy))
	if !ok || err1 != nil || err2 != nil || m < 1 || m > 12 {
		return "", fmt.Errorf("%w: Expiry must be MM/YY.", errPayment)
	}
	if y < 100 {
		y += 2000
	}
	// card is valid through the last day of its expiry month
	if !now.Before(time.Date(y, time.Month(m)+1, 1, 0, 0, 0, 0, time.UTC)) {
		return "", fmt.Errorf("%w: This card has expired.", errPayment)
	}
	if len(p.CVC) < 3 || len(p.CVC) > 4 || strings.Trim(p.CVC, "0123456789") != "" {
		return "", fmt.Errorf("%w: CVC must be 3 or 4 digits.", errPayment)
	}
	switch num {
	case "4000000000000002":
		return "", fmt.Errorf("%w: Your card was declined.", errPayment)
	case "4000000000009995":
		return "", fmt.Errorf("%w: Insufficient funds.", errPayment)
	}
	if amountCents <= 0 {
		return "", fmt.Errorf("%w: Nothing to pay for.", errPayment)
	}
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("sim_%x", b), nil
}

const keyAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I confusion

// generateKey makes a product-key-shaped code, e.g. "7KQ2M-XH4PD-9WZ3A".
func generateKey() string {
	b := make([]byte, 15)
	rand.Read(b)
	var s strings.Builder
	for i, v := range b {
		if i > 0 && i%5 == 0 {
			s.WriteByte('-')
		}
		s.WriteByte(keyAlphabet[int(v)%len(keyAlphabet)])
	}
	return s.String()
}

type OrderItem struct {
	ProductSlug string `json:"productSlug"`
	ProductName string `json:"productName"`
	CoverURL    string `json:"coverUrl"`
	SellerName  string `json:"sellerName"`
	PriceCents  int    `json:"priceCents"`
	KeyCode     string `json:"keyCode"`
}

type Order struct {
	ID         int         `json:"id"`
	TotalCents int         `json:"totalCents"`
	Status     string      `json:"status"`
	PaymentRef string      `json:"paymentRef"`
	CardLast4  string      `json:"cardLast4"`
	CreatedAt  string      `json:"createdAt"`
	Items      []OrderItem `json:"items"`
}

type checkoutError struct {
	status int
	msg    string
}

func (e *checkoutError) Error() string { return e.msg }

// POST /api/checkout (signed-in only)
// Everything happens in one DB transaction: re-price from the server, reserve stock
// (conditional UPDATE so two buyers can't get the last key), charge, then issue one
// unique key per unit. If payment fails the transaction rolls back and stock is restored.
func (a *App) handleCheckout(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r)
	var body struct {
		Items          []CartLine   `json:"items"`
		Payment        PaymentInput `json:"payment"`
		IdempotencyKey string       `json:"idempotencyKey"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	if len(body.Items) == 0 {
		writeErr(w, 400, "Your cart is empty.")
		return
	}

	// Retried request (double click, flaky network): return the order already created.
	if body.IdempotencyKey != "" {
		var existing int
		if err := a.db.QueryRow(`SELECT id FROM orders WHERE idempotency_key = ? AND user_id = ?`, body.IdempotencyKey, user.ID).Scan(&existing); err == nil {
			o, err := a.loadOrder(existing, user.ID)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, o)
			return
		}
	}

	orderID, err := a.placeOrder(user.ID, body.Items, body.Payment, body.IdempotencyKey)
	if err != nil {
		var ce *checkoutError
		if errors.As(err, &ce) {
			writeErr(w, ce.status, ce.msg)
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	o, err := a.loadOrder(orderID, user.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, o)
}

func (a *App) placeOrder(userID int, items []CartLine, pay PaymentInput, idem string) (int, error) {
	// merge duplicate lines
	merged := map[int]int{}
	order := []int{}
	for _, it := range items {
		if it.Quantity < 1 || it.Quantity > 10 {
			return 0, &checkoutError{400, "Quantity must be between 1 and 10."}
		}
		if _, ok := merged[it.OfferID]; !ok {
			order = append(order, it.OfferID)
		}
		merged[it.OfferID] += it.Quantity
	}
	lines := make([]CartLine, 0, len(order))
	for _, id := range order {
		lines = append(lines, CartLine{OfferID: id, Quantity: merged[id]})
	}

	tx, err := a.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	quoted, total, err := a.quote(tx, lines)
	if err != nil {
		return 0, err
	}
	if len(quoted) != len(lines) {
		return 0, &checkoutError{409, "Some items in your cart no longer exist. Please review your cart."}
	}
	for _, l := range quoted {
		res, err := tx.Exec(`UPDATE offers SET stock = stock - ? WHERE id = ? AND stock >= ?`, l.Quantity, l.OfferID, l.Quantity)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return 0, &checkoutError{409, fmt.Sprintf("Not enough stock for %s from %s.", l.ProductName, l.SellerName)}
		}
	}

	ref, err := chargeCard(pay, total, time.Now().UTC())
	if err != nil {
		return 0, &checkoutError{402, strings.TrimPrefix(err.Error(), "payment: ")}
	}
	num := strings.ReplaceAll(strings.ReplaceAll(pay.CardNumber, " ", ""), "-", "")
	last4 := num[len(num)-4:]

	var idemVal any
	if idem != "" {
		idemVal = idem
	}
	res, err := tx.Exec(`INSERT INTO orders(user_id,total_cents,status,payment_ref,card_last4,idempotency_key) VALUES(?,?,?,?,?,?)`,
		userID, total, "completed", ref, last4, idemVal)
	if err != nil {
		return 0, err
	}
	orderID64, _ := res.LastInsertId()
	for _, l := range quoted {
		var productID int
		if err := tx.QueryRow(`SELECT product_id FROM offers WHERE id = ?`, l.OfferID).Scan(&productID); err != nil {
			return 0, err
		}
		for i := 0; i < l.Quantity; i++ {
			if _, err := tx.Exec(`INSERT INTO order_items(order_id,offer_id,product_id,seller_name,product_name,price_cents,key_code) VALUES(?,?,?,?,?,?,?)`,
				orderID64, l.OfferID, productID, l.SellerName, l.ProductName, l.PriceCents, generateKey()); err != nil {
				return 0, err
			}
		}
		// feeds the "Best sellers" homepage block
		if _, err := tx.Exec(`UPDATE products SET sales_count = sales_count + ? WHERE id = ?`, l.Quantity, productID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(orderID64), nil
}

func (a *App) loadOrder(id, userID int) (*Order, error) {
	var o Order
	err := a.db.QueryRow(`SELECT id, total_cents, status, payment_ref, card_last4, created_at FROM orders WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&o.ID, &o.TotalCents, &o.Status, &o.PaymentRef, &o.CardLast4, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	rows, err := a.db.Query(`SELECT p.slug, oi.product_name, oi.seller_name, oi.price_cents, oi.key_code
		FROM order_items oi JOIN products p ON p.id = oi.product_id WHERE oi.order_id = ? ORDER BY oi.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	o.Items = []OrderItem{}
	for rows.Next() {
		var it OrderItem
		if err := rows.Scan(&it.ProductSlug, &it.ProductName, &it.SellerName, &it.PriceCents, &it.KeyCode); err != nil {
			return nil, err
		}
		it.CoverURL = "/api/covers/" + it.ProductSlug
		o.Items = append(o.Items, it)
	}
	return &o, rows.Err()
}

func (a *App) handleOrders(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r)
	rows, err := a.db.Query(`SELECT id FROM orders WHERE user_id = ? ORDER BY id DESC`, user.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	out := []*Order{}
	for _, id := range ids {
		o, err := a.loadOrder(id, user.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out = append(out, o)
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (a *App) handleOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	o, err := a.loadOrder(id, userFrom(r).ID)
	if err == sql.ErrNoRows {
		writeErr(w, 404, "order not found") // also when it belongs to someone else
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, o)
}
