package database

import (
	"fmt"
	"strconv"
	"strings"
)

// CollectionPage is request pagination, not a resource model. Each resource
// supplies its own query, columns, authorization, and native result type.
type CollectionPage struct {
	Name   string
	Cursor string
	Limit  int
}

func (store *Store) collectionPageSQL(pages []CollectionPage, id string, columns []string, args []any, legacyOrder string, integerCursor ...bool) (string, []any, error) {
	if len(pages) == 0 || pages[0].Limit == 0 {
		return " ORDER BY " + legacyOrder, args, nil
	}
	page := pages[0]
	if page.Limit < 1 || page.Limit > 100 || len(page.Name) > 256 {
		return "", nil, fmt.Errorf("invalid collection page")
	}
	p := keychainPlaceholder(store.kind)
	query := ""
	if page.Name != "" {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(page.Name)) + "%"
		matches := []string{}
		for _, column := range append([]string{id}, columns...) {
			args = append(args, pattern)
			matches = append(matches, "LOWER(COALESCE(CAST("+column+" AS TEXT), '')) LIKE LOWER("+p(len(args))+`) ESCAPE '\'`)
		}
		query += " AND (" + strings.Join(matches, " OR ") + ")"
	}
	if page.Cursor != "" {
		var cursor any = page.Cursor
		if len(integerCursor) > 0 && integerCursor[0] {
			value, err := strconv.Atoi(page.Cursor)
			if err != nil || value < 1 {
				return "", nil, fmt.Errorf("invalid revision cursor")
			}
			cursor = value
		}
		args = append(args, cursor)
		query += " AND " + id + " < " + p(len(args))
	}
	args = append(args, page.Limit+1)
	return query + " ORDER BY " + id + " DESC LIMIT " + p(len(args)), args, nil
}
