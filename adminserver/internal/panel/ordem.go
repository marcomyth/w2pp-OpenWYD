package panel

import (
	"net/http"
	"net/url"
)

// Table sorting, by link.
//
// The panel serves no JavaScript, so a sortable column cannot be a click
// handler: it is a link back to the same page carrying the order in the query
// string. That is not a workaround — it is better in two ways nobody planned.
// The order survives being shared or bookmarked, and the back button undoes it.
//
// What it replaces: every list in whatever order the source happened to return,
// and a moderator scanning forty rows by eye for the biggest number.

// ordem is a table's current sort.
type ordem struct {
	// Por is the column key, or "" for the page's own default.
	Por string
	// Desc reverses it. Two directions per column and no third "no sort" state:
	// a table is always in SOME order, and pretending otherwise gives the header
	// a click that appears to do nothing.
	Desc bool
}

// ordemDe reads the sort out of a request, accepting only the keys the page
// offers.
//
// An unknown key falls back to the page default instead of erroring: it arrives
// from an old bookmark or a typed URL, and refusing the page over a column that
// no longer exists would be a dead end where there was still something to show.
func ordemDe(r *http.Request, permitidas ...string) ordem {
	o := ordem{Por: r.URL.Query().Get("ordem"), Desc: r.URL.Query().Get("desc") != ""}
	for _, p := range permitidas {
		if o.Por == p {
			return o
		}
	}
	return ordem{Desc: o.Desc}
}

// Link builds the href for one column header, keeping whatever else the page
// carries in its query string — a search term above all, which would otherwise
// be thrown away by the act of sorting.
//
// Clicking the column you are already sorted by flips the direction; clicking
// another starts it ascending. That is what every table anyone has used does,
// and the surprise of it behaving otherwise costs more than the code.
func (o ordem) Link(base, coluna string, extras url.Values) string {
	v := url.Values{}
	for k, vals := range extras {
		if k == "ordem" || k == "desc" {
			continue // these are what this link is setting
		}
		for _, val := range vals {
			if val != "" {
				v.Add(k, val)
			}
		}
	}
	v.Set("ordem", coluna)
	if o.Por == coluna && !o.Desc {
		v.Set("desc", "1")
	}
	return base + "?" + v.Encode()
}

// Marca reports how one header should be drawn: "asc", "desc", or "" for a
// column that is not the current sort.
func (o ordem) Marca(coluna string) string {
	if o.Por != coluna {
		return ""
	}
	if o.Desc {
		return "desc"
	}
	return "asc"
}

// Menor applies the direction to a page's ascending comparator, so no page
// writes the same comparison twice to get both ways.
//
// Callers pass it to sort.SliceStable rather than sort.Slice: rows that tie must
// stay in the order the source produced, or two loads of the same page disagree
// about equal rows and the list reads as shuffling on its own.
func (o ordem) Menor(asc func(i, j int) bool) func(i, j int) bool {
	if o.Desc {
		return func(i, j int) bool { return asc(j, i) }
	}
	return asc
}
