package main

import (
	"database/sql"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Typo-tolerant search over product names.
//
// The catalog is small (a few hundred listings), so an in-memory index rebuilt
// at startup is simpler and faster than an external engine. Each listing is
// broken into tokens; every query token is matched against every listing token
// with a tiered score (exact > prefix > substring > edit distance). This covers:
//   - partial names:  "cyber"        -> Cyberpunk 2077
//   - typos:          "eldn rnig"    -> Elden Ring
//   - glued words:    "eldenring"    -> Elden Ring
//   - acronyms:       "gta", "rdr2"  -> Grand Theft Auto V, Red Dead Redemption 2

type indexedProduct struct {
	id          int
	title       string
	titleTokens []string
	realTitle   int      // number of real title words (titleTokens also holds aliases)
	metaTokens  []string // platform, device, region, type, genre
	compact     string   // title with all separators removed
	sales       int
}

type SearchIndex struct {
	mu    sync.RWMutex
	items []indexedProduct
}

type SearchHit struct {
	ID    int
	Score float64
	Fuzzy bool // true when the best match needed typo correction
}

func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r == '\'' || r == '’' || r == ':' || r == '.':
			// drop: "Baldur's" -> "baldurs", "Clair Obscur:" -> "clair obscur"
		case r == 'ö':
			b.WriteRune('o')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func tokens(s string) []string { return strings.Fields(normalize(s)) }

var romanNumerals = map[string]string{"ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7"}

func buildIndex(db *sql.DB) (*SearchIndex, error) {
	rows, err := db.Query(`SELECT id, title, platform, device, region, type, genre, sales_count FROM products`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	idx := &SearchIndex{}
	for rows.Next() {
		var p indexedProduct
		var platform, device, region, typ, genre string
		if err := rows.Scan(&p.id, &p.title, &platform, &device, &region, &typ, &genre, &p.sales); err != nil {
			return nil, err
		}
		tt := tokens(p.title)
		p.realTitle = len(tt)
		// Acronym ("gtav", "rdr2") and roman numeral aliases ("vii" -> "7").
		if len(tt) >= 2 {
			var acr strings.Builder
			for _, t := range tt {
				if _, isNum := romanNumerals[t]; isNum || isAllDigits(t) {
					acr.WriteString(t)
				} else {
					acr.WriteByte(t[0])
				}
			}
			tt = append(tt, acr.String())
		}
		for _, t := range tt {
			if n, ok := romanNumerals[t]; ok {
				tt = append(tt, n)
			}
		}
		p.titleTokens = tt
		p.metaTokens = tokens(strings.Join([]string{platform, device, region, typ, genre}, " "))
		if strings.Contains(strings.ToLower(platform), "psn") {
			p.metaTokens = append(p.metaTokens, "playstation", "ps")
		}
		if strings.Contains(strings.ToLower(platform), "nintendo") {
			p.metaTokens = append(p.metaTokens, "switch")
		}
		p.compact = strings.ReplaceAll(normalize(p.title), " ", "")
		idx.items = append(idx.items, p)
	}
	return idx, rows.Err()
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// damerauLevenshtein (optimal string alignment) distance, capped by max for speed.
func damerauLevenshtein(a, b string, max int) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if abs(la-lb) > max {
		return max + 1
	}
	prev2 := make([]int, lb+1)
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
			rowMin = min(rowMin, cur[j])
		}
		if rowMin > max {
			return max + 1
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[lb]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func allowedTypos(n int) int {
	switch {
	case n <= 2:
		return 0
	case n <= 5:
		return 1
	default:
		return 2
	}
}

// tokenScore returns how well query token q matches listing token t (0..1) and whether it was fuzzy.
func tokenScore(q, t string) (float64, bool) {
	if q == t {
		return 1.0, false
	}
	if strings.HasPrefix(t, q) {
		// short prefixes are weak signals ("e" matches everything)
		if len(q) == 1 {
			return 0.35, false
		}
		return 0.75 + 0.2*float64(len(q))/float64(len(t)), false
	}
	if len(q) >= 3 && strings.Contains(t, q) {
		return 0.6, false
	}
	maxD := allowedTypos(len(q))
	if maxD == 0 {
		return 0, false
	}
	if d := damerauLevenshtein(q, t, maxD); d <= maxD {
		return 0.8 - 0.15*float64(d), true
	}
	// typo inside a partial word: compare with a prefix of the same length ("witchr" ~ "witcher")
	if len(t) > len(q) {
		if d := damerauLevenshtein(q, t[:len(q)], maxD); d <= maxD && d < len(q)/2 {
			return 0.65 - 0.15*float64(d), true
		}
	}
	return 0, false
}

func (idx *SearchIndex) Search(query string) []SearchHit {
	qt := tokens(query)
	if len(qt) == 0 {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var hits []SearchHit
	for _, p := range idx.items {
		total := 0.0
		fuzzy := false
		misses := 0
		covered := map[int]bool{}
		for _, q := range qt {
			best, bestFuzzy, bestTok := 0.0, false, -1
			for ti, t := range p.titleTokens {
				if s, f := tokenScore(q, t); s > best {
					best, bestFuzzy, bestTok = s, f, ti
				}
			}
			for _, t := range p.metaTokens {
				// metadata matches (platform/region) count slightly less than title matches
				if s, f := tokenScore(q, t); s*0.85 > best {
					best, bestFuzzy, bestTok = s*0.85, f, -1
				}
			}
			if bestTok >= 0 {
				if bestTok >= p.realTitle {
					covered[-1] = true // alias (acronym) covers the whole title
				} else {
					covered[bestTok] = true
				}
			}
			if best == 0 && len(q) >= 4 && strings.Contains(p.compact, q) {
				best = 0.7 // glued words: "eldenring", "hollowknight"
			}
			if best == 0 {
				misses++
			}
			total += best
			fuzzy = fuzzy || bestFuzzy
		}
		// Every query token must match, except we forgive one miss in long queries.
		if misses > 0 && !(len(qt) >= 3 && misses == 1) {
			continue
		}
		score := total / float64(len(qt))
		// Prefer listings whose title is fully covered by the query:
		// "elden ring" should rank the base game above "Elden Ring Shadow of the Erdtree".
		coverage := float64(len(covered)) / float64(max(p.realTitle, 1))
		if covered[-1] {
			coverage = 1
		}
		score = score*0.85 + 0.15*min(coverage, 1)
		if score < 0.3 {
			continue
		}
		// Whole query glued equals the title -> strong match.
		if strings.ReplaceAll(normalize(query), " ", "") == p.compact {
			score = 1.0
		}
		hits = append(hits, SearchHit{ID: p.id, Score: score, Fuzzy: fuzzy})
	}
	sales := map[int]int{}
	for _, p := range idx.items {
		sales[p.id] = p.sales
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if d := hits[i].Score - hits[j].Score; d > 0.001 || d < -0.001 {
			return d > 0
		}
		return sales[hits[i].ID] > sales[hits[j].ID]
	})
	return hits
}

// Suggest returns the base title of the best hit when the query looked misspelled,
// so the UI can show "Showing results for ...".
func (idx *SearchIndex) Suggest(hits []SearchHit) string {
	if len(hits) == 0 || !hits[0].Fuzzy {
		return ""
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	for _, p := range idx.items {
		if p.id == hits[0].ID {
			return p.title
		}
	}
	return ""
}
