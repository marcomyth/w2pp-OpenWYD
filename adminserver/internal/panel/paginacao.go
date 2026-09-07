package panel

import (
	"net/http"
	"net/url"
	"strconv"
)

// Pagination, by link.
//
// What it replaces: a fixed cap that cut the list and — at best — said so. The
// message was honest, and it was still a dead end: "showing the 100 most
// recent" with no way to see the 101st. For a queue that is fine, for a history
// it is not, and the audit trail and the chat log are histories.
//
// Like the sorting next door, this is links rather than script, so a page deep
// in a list can be shared and the back button walks out of it.

// paginaTam is how many rows fit on one page.
//
// Not configurable from the URL. A ?tamanho= would let anyone ask for fifty
// thousand rows in one query, which is a denial of service written by the
// person it inconveniences.
const paginaTam = 50

// pagina is which slice of a long list is on screen.
type pagina struct {
	// N is 1-based, because it appears in a URL people read and type.
	N int
	// Param is the query key this page reads and writes. It exists because one
	// screen can carry two lists — the trades page has the trade window and the
	// floor — and a single "pagina" would move both at once, which is never what
	// somebody turning one of them meant.
	Param string
	// Tam is the page size, carried so a template does not have to know it.
	Tam int
	// TemMais says a next page exists.
	//
	// It comes from fetching ONE row more than the page holds and finding it,
	// not from a COUNT(*). The count would be a second query over the whole
	// table whose answer is already stale by the time it renders, and all the
	// screen needs to know is whether to draw a "next" link.
	TemMais bool
}

// paginaDe reads the page number out of a request.
//
// Anything unreadable, zero or negative becomes page 1 rather than an error:
// it arrives from a typed URL or an old link, and refusing the page over it
// would be a dead end where there was a perfectly good first page to show.
func paginaDe(r *http.Request, param string) pagina {
	if param == "" {
		param = "pagina"
	}
	p := pagina{N: 1, Tam: paginaTam, Param: param}
	if n, err := strconv.Atoi(r.URL.Query().Get(param)); err == nil && n > 1 {
		// A ceiling, so a typed ?pagina=99999999 does not turn into an OFFSET
		// the database walks row by row.
		if n > 500 {
			n = 500
		}
		p.N = n
	}
	return p
}

// Offset is where this page starts in the full list.
func (p pagina) Offset() int { return (p.N - 1) * p.Tam }

// Pedir is how many rows to ask the store for: one more than fits, which is how
// TemMais gets answered without counting anything.
func (p pagina) Pedir() int { return p.Tam + 1 }

// Corta trims the extra row and records whether it was there. Callers pass the
// slice they got back from a store queried with Pedir.
func Corta[T any](p *pagina, linhas []T) []T {
	if len(linhas) > p.Tam {
		p.TemMais = true
		return linhas[:p.Tam]
	}
	p.TemMais = false
	return linhas
}

// Primeira reports whether this is page one, so the template can leave the
// "previous" link out rather than draw a dead control.
func (p pagina) Primeira() bool { return p.N <= 1 }

// De and Ate are the row numbers on screen, 1-based and inclusive of De. They
// exist so the page can say "51 a 100" instead of "page 2", which is the thing
// somebody comparing two screens actually wants to know.
func (p pagina) De() int { return p.Offset() + 1 }

// Link builds the href for another page, keeping the rest of the query string —
// the search and the sort above all, which would otherwise be thrown away by
// the act of turning a page.
func (p pagina) Link(base string, n int, extras url.Values) string {
	chave := p.Param
	if chave == "" {
		chave = "pagina"
	}
	v := url.Values{}
	for k, vals := range extras {
		if k == chave {
			continue
		}
		for _, val := range vals {
			if val != "" {
				v.Add(k, val)
			}
		}
	}
	if n > 1 {
		v.Set(chave, strconv.Itoa(n))
	}
	if len(v) == 0 {
		return base
	}
	return base + "?" + v.Encode()
}

// Anterior and Proxima are the neighbouring page numbers, for the template.
func (p pagina) Anterior() int { return p.N - 1 }
func (p pagina) Proxima() int  { return p.N + 1 }
