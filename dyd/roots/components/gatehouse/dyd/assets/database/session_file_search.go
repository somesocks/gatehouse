package database

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"gatehouse/inputform"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var ErrSessionFileSearch = errors.New("invalid session file search")

type SessionFileSearch struct {
	Name      string
	MediaType string
	Accept    []string
	Cursor    string
	Direction string
	Limit     int
}

type SessionFilePage struct {
	Files          []SessionFileSummary `json:"files"`
	NextCursor     string               `json:"next_cursor,omitempty"`
	PreviousCursor string               `json:"previous_cursor,omitempty"`
}

func (search SessionFileSearch) Validate() error {
	if search.Limit < 1 || search.Limit > 100 || len(search.Name) > 1024 || len(search.Accept) > 32 || search.Cursor != "" && !typed_id.Valid(typed_id.SessionFile, search.Cursor) || search.Direction != "next" && search.Direction != "previous" || search.Direction == "previous" && search.Cursor == "" {
		return ErrSessionFileSearch
	}
	if search.MediaType != "" && (len(search.MediaType) > 255 || !inputform.ValidMediaTypePattern(search.MediaType)) {
		return ErrSessionFileSearch
	}
	for _, mediaType := range search.Accept {
		if len(mediaType) > 255 || !inputform.ValidMediaTypePattern(mediaType) {
			return ErrSessionFileSearch
		}
	}
	return nil
}

// SessionFilesSearch uses the session/id primary-key index for keyset paging.
// Both filters and LIMIT are applied in SQL; no full file collection is loaded.
func (store *Store) SessionFilesSearch(ctx context.Context, session model.SessionRef, search SessionFileSearch) (error, SessionFilePage) {
	page := SessionFilePage{Files: []SessionFileSummary{}}
	if err := search.Validate(); err != nil {
		return err, page
	}
	p := keychainPlaceholder(store.kind)
	args := []any{session.Workspace.Id, session.Id}
	bind := func(value any) string { args = append(args, value); return p(len(args)) }
	conditions := []string{"files.workspace = " + p(1), "files.session = " + p(2), "files.enabled = TRUE", "objects.state = 'success'"}
	if strings.TrimSpace(search.Name) != "" {
		name := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(search.Name)) + "%"
		conditions = append(conditions, "LOWER(files.name) LIKE LOWER("+bind(name)+") ESCAPE '\\'")
	}
	mediaCondition := func(mediaType string) string {
		if strings.HasSuffix(mediaType, "/*") {
			prefix := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSuffix(mediaType, "*")) + "%"
			return "LOWER(files.media_type) LIKE " + bind(prefix) + " ESCAPE '\\'"
		}
		return "LOWER(files.media_type) = " + bind(mediaType)
	}
	if len(search.Accept) > 0 {
		accepted := []string{}
		for _, mediaType := range search.Accept {
			accepted = append(accepted, mediaCondition(mediaType))
		}
		conditions = append(conditions, "("+strings.Join(accepted, " OR ")+")")
	}
	if search.MediaType != "" {
		conditions = append(conditions, mediaCondition(search.MediaType))
	}
	base := strings.Join(conditions, " AND ")
	baseArgs := append([]any{}, args...)
	order := "DESC"
	if search.Cursor != "" {
		comparison := "<"
		if search.Direction == "previous" {
			comparison = ">"
			order = "ASC"
		}
		conditions = append(conditions, "files.id "+comparison+" "+bind(search.Cursor))
	}
	query := `SELECT files.id, files.name, files.media_type, objects.size, objects.sha256
		FROM gatehouse_session_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object
		WHERE ` + strings.Join(conditions, " AND ") + ` ORDER BY files.id ` + order + ` LIMIT ` + bind(search.Limit)
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("search session files: %w", err), page
	}
	for rows.Next() {
		file, err := scanSessionFileSummary(rows)
		if err != nil {
			rows.Close()
			return err, page
		}
		page.Files = append(page.Files, file)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err, page
	}
	if search.Direction == "previous" {
		for left, right := 0, len(page.Files)-1; left < right; left, right = left+1, right-1 {
			page.Files[left], page.Files[right] = page.Files[right], page.Files[left]
		}
	}
	if len(page.Files) == 0 {
		return nil, page
	}
	first, last := page.Files[0].ID, page.Files[len(page.Files)-1].ID
	var newer, older bool
	for _, boundary := range []struct {
		comparison, id string
		exists         *bool
	}{{">", first, &newer}, {"<", last, &older}} {
		query = `SELECT EXISTS (SELECT 1 FROM gatehouse_session_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object WHERE ` + base + ` AND files.id ` + boundary.comparison + ` ` + p(len(baseArgs)+1) + `)`
		if err := store.QueryRowContext(ctx, query, append(append([]any{}, baseArgs...), boundary.id)...).Scan(boundary.exists); err != nil {
			return err, page
		}
	}
	if newer {
		page.PreviousCursor = first
	}
	if older {
		page.NextCursor = last
	}
	return nil, page
}

// SessionFilesByIDs hydrates only the selected references, in bounded batches.
// Removed or unfinished files are omitted so the client can show them as unavailable.
func (store *Store) SessionFilesByIDs(ctx context.Context, session model.SessionRef, ids []string) (error, []SessionFileSummary) {
	if len(ids) > 100 {
		return ErrSessionFileSearch, nil
	}
	if len(ids) == 0 {
		return nil, []SessionFileSummary{}
	}
	p := keychainPlaceholder(store.kind)
	args := []any{session.Workspace.Id, session.Id}
	parameters := []string{}
	for _, id := range ids {
		if !typed_id.Valid(typed_id.SessionFile, id) {
			return ErrSessionFileSearch, nil
		}
		args = append(args, id)
		parameters = append(parameters, p(len(args)))
	}
	rows, err := store.QueryContext(ctx, `SELECT files.id,files.name,files.media_type,objects.size,objects.sha256 FROM gatehouse_session_files AS files JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object WHERE files.workspace = `+p(1)+` AND files.session = `+p(2)+` AND files.enabled = TRUE AND objects.state = 'success' AND files.id IN (`+strings.Join(parameters, ",")+`)`, args...)
	if err != nil {
		return err, nil
	}
	defer rows.Close()
	byID := map[string]SessionFileSummary{}
	for rows.Next() {
		file, err := scanSessionFileSummary(rows)
		if err != nil {
			return err, nil
		}
		byID[file.ID] = file
	}
	if err := rows.Err(); err != nil {
		return err, nil
	}
	files := []SessionFileSummary{}
	for _, id := range ids {
		if file, ok := byID[id]; ok {
			files = append(files, file)
		}
	}
	return nil, files
}

func scanSessionFileSummary(row interface{ Scan(...any) error }) (SessionFileSummary, error) {
	var file SessionFileSummary
	var mediaType sql.NullString
	var digest []byte
	err := row.Scan(&file.ID, &file.Name, &mediaType, &file.Size, &digest)
	if mediaType.Valid {
		file.MediaType = &mediaType.String
	}
	file.Fingerprint = "sha256:" + hex.EncodeToString(digest)
	return file, err
}
