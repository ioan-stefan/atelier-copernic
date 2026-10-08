package main

import (
	"cmp"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

const maxQueryRunes = 80

// LedgerQuery is the validated state of the ledger's search form. It lives
// entirely in the URL, so every view of the ledger can be bookmarked.
type LedgerQuery struct {
	Q          string
	Hemisphere string
	Tier       string
	Sort       string
	Dir        string
}

var ledgerSorts = map[string]func(a, b *LedgerEntry) int{
	"ref":      func(a, b *LedgerEntry) int { return strings.Compare(a.Ref, b.Ref) },
	"name":     func(a, b *LedgerEntry) int { return strings.Compare(a.Name, b.Name) },
	"year":     func(a, b *LedgerEntry) int { return cmp.Compare(a.Year, b.Year) },
	"caliber":  func(a, b *LedgerEntry) int { return strings.Compare(a.Code, b.Code) },
	"tier":     func(a, b *LedgerEntry) int { return cmp.Compare(tierRank(a.TierID), tierRank(b.TierID)) },
	"latitude": func(a, b *LedgerEntry) int { return cmp.Compare(a.SignedLatitude(), b.SignedLatitude()) },
	"alloy":    func(a, b *LedgerEntry) int { return strings.Compare(a.AlloyCode, b.AlloyCode) },
}

// ParseLedgerQuery whitelists every parameter; anything unknown falls back to
// the default view rather than erroring.
func ParseLedgerQuery(v url.Values) LedgerQuery {
	q := LedgerQuery{
		Q:          strings.TrimSpace(v.Get("q")),
		Hemisphere: v.Get("hemisphere"),
		Tier:       v.Get("tier"),
		Sort:       v.Get("sort"),
		Dir:        v.Get("dir"),
	}
	if utf8.RuneCountInString(q.Q) > maxQueryRunes {
		q.Q = string([]rune(q.Q)[:maxQueryRunes])
	}
	if q.Hemisphere != string(Northern) && q.Hemisphere != string(Southern) {
		q.Hemisphere = ""
	}
	if _, ok := tierByID(q.Tier); !ok {
		q.Tier = ""
	}
	if _, ok := ledgerSorts[q.Sort]; !ok {
		q.Sort = "year"
	}
	if q.Dir != "desc" {
		q.Dir = "asc"
	}
	return q
}

// Filtered reports whether the query narrows the ledger at all.
func (q LedgerQuery) Filtered() bool {
	return q.Q != "" || q.Hemisphere != "" || q.Tier != ""
}

// URL returns the ledger URL for this query with a different sort.
func (q LedgerQuery) URL(sort, dir string) string {
	v := url.Values{}
	if q.Q != "" {
		v.Set("q", q.Q)
	}
	if q.Hemisphere != "" {
		v.Set("hemisphere", q.Hemisphere)
	}
	if q.Tier != "" {
		v.Set("tier", q.Tier)
	}
	v.Set("sort", sort)
	v.Set("dir", dir)
	return "/ledger?" + v.Encode()
}

// SearchLedger filters and sorts a copy of the ledger.
func (c *Catalog) SearchLedger(q LedgerQuery) []LedgerEntry {
	terms := strings.Fields(fold(q.Q))
	out := make([]LedgerEntry, 0, len(c.Ledger))
	for _, e := range c.Ledger {
		if q.Hemisphere != "" && string(e.Hemisphere) != q.Hemisphere {
			continue
		}
		if q.Tier != "" && e.TierID != q.Tier {
			continue
		}
		if !matchesAll(e.search, terms) {
			continue
		}
		out = append(out, e)
	}
	by := ledgerSorts[q.Sort]
	slices.SortStableFunc(out, func(a, b LedgerEntry) int {
		r := by(&a, &b)
		if r == 0 {
			r = strings.Compare(a.Ref, b.Ref)
		}
		if q.Dir == "desc" {
			return -r
		}
		return r
	})
	return out
}

func matchesAll(haystack string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(haystack, t) {
			return false
		}
	}
	return true
}

// SortColumn is one sortable header of the ledger table.
type SortColumn struct {
	Key   string
	Label string
	URL   string
	Aria  string // aria-sort value, empty when the column is not sorted
	Mark  string
}

var ledgerColumnDefs = []struct{ key, label string }{
	{"ref", "Reference"},
	{"name", "Instrument"},
	{"year", "Year"},
	{"caliber", "Caliber"},
	{"tier", "Tier"},
	{"latitude", "Latitude"},
	{"alloy", "Alloy"},
}

func ledgerColumns(q LedgerQuery) []SortColumn {
	cols := make([]SortColumn, len(ledgerColumnDefs))
	for i, d := range ledgerColumnDefs {
		col := SortColumn{Key: d.key, Label: d.label, URL: q.URL(d.key, "asc")}
		if q.Sort == d.key {
			if q.Dir == "asc" {
				col.Aria, col.Mark, col.URL = "ascending", "↑", q.URL(d.key, "desc")
			} else {
				col.Aria, col.Mark = "descending", "↓"
			}
		}
		cols[i] = col
	}
	return cols
}

func sortLabel(key string) string {
	for _, d := range ledgerColumnDefs {
		if d.key == key {
			return strings.ToLower(d.label)
		}
	}
	return key
}
