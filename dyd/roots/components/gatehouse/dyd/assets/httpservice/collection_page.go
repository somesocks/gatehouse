package httpservice

import (
	"net/http"
	"strconv"
	"strings"

	"gatehouse/database"
	"gatehouse/typed_id"
)

func collectionPageParameters(w http.ResponseWriter, r *http.Request, kind string) ([]database.CollectionPage, bool) {
	if !r.URL.Query().Has("limit") {
		return nil, true
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	page := database.CollectionPage{Name: strings.TrimSpace(r.URL.Query().Get("name")), Cursor: r.URL.Query().Get("cursor"), Limit: limit}
	validCursor := page.Cursor == "" || typed_id.Valid(kind, page.Cursor)
	if kind == "" && page.Cursor != "" {
		revision, parseErr := strconv.Atoi(page.Cursor)
		validCursor = parseErr == nil && revision > 0 && strconv.Itoa(revision) == page.Cursor
	}
	if err != nil || limit < 1 || limit > 100 || len(page.Name) > 256 || !validCursor {
		http.Error(w, "invalid collection search", http.StatusBadRequest)
		return nil, false
	}
	return []database.CollectionPage{page}, true
}

// Existing endpoints retain their native array bodies. The continuation header
// is present only when another bounded page exists.
func writeCollectionPage[T any](w http.ResponseWriter, items []T, pages []database.CollectionPage, id func(T) string) {
	if len(pages) > 0 && len(items) > pages[0].Limit {
		items = items[:pages[0].Limit]
		w.Header().Set("X-Next-Cursor", id(items[len(items)-1]))
	}
	writeJSON(w, items)
}
