package httpservice

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"testing"

	"gatehouse/database"
	"gatehouse/inputform"
)

func TestCustomInputFilesPagedAndFiltered(t *testing.T) {
	fixture := newInputTestFixture(t, inputform.Field{ID: "files", Label: "Files", Type: "custom", Custom: &inputform.CustomField{URL: "/app/tools/session-file-picker/", Inputs: json.RawMessage(`{}`), Capabilities: []string{inputform.SessionFileList, inputform.SessionFileUpload}}})
	token := customFieldToken(t, fixture, formTokenForFixture(t, fixture), []string{"files"})
	ids := []string{}
	for _, file := range []struct{ name, mediaType string }{{"report%_.txt", "text/plain"}, {"report_a.txt", "text/plain"}, {"REPORT-B.txt", "text/plain"}, {"report-c.txt", "text/plain"}, {"photo.png", "image/png"}, {"image.svg", "image/svg+xml"}, {"document.pdf", "application/pdf"}} {
		body, _ := json.Marshal(map[string]string{"name": file.name, "media_type": file.mediaType})
		created := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/field/files", token, string(body))
		var upload sessionFileCreateResponse
		if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &upload) != nil {
			t.Fatalf("create=%d %s", created.Code, created.Body.String())
		}
		if put := inputTestRequest(fixture.handler, http.MethodPut, upload.UploadURL, "", "data"); put.Code != http.StatusNoContent {
			t.Fatalf("put=%d", put.Code)
		}
		if finish := inputTestRequest(fixture.handler, http.MethodPost, "/api/v1/input/field/files/"+upload.File.Ref.Id+"/finish", token, ""); finish.Code != http.StatusOK {
			t.Fatalf("finish=%d %s", finish.Code, finish.Body.String())
		}
		ids = append(ids, upload.File.Ref.Id)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	read := func(query url.Values) database.SessionFilePage {
		t.Helper()
		response := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field/files?"+query.Encode(), token, "")
		var page database.SessionFilePage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatalf("page=%d %s", response.Code, response.Body.String())
		}
		return page
	}
	pageIDs := func(page database.SessionFilePage) []string {
		out := []string{}
		for _, file := range page.Files {
			out = append(out, file.ID)
		}
		return out
	}
	first := read(url.Values{"limit": {"2"}})
	if !reflect.DeepEqual(pageIDs(first), ids[:2]) || first.NextCursor == "" || first.PreviousCursor != "" {
		t.Fatalf("first page=%#v", first)
	}
	second := read(url.Values{"limit": {"2"}, "cursor": {first.NextCursor}})
	if !reflect.DeepEqual(pageIDs(second), ids[2:4]) || second.NextCursor == "" || second.PreviousCursor == "" {
		t.Fatalf("second page=%#v", second)
	}
	previous := read(url.Values{"limit": {"2"}, "cursor": {second.PreviousCursor}, "direction": {"previous"}})
	if !reflect.DeepEqual(pageIDs(previous), pageIDs(first)) {
		t.Fatalf("previous page=%#v", previous)
	}
	all := append(pageIDs(first), pageIDs(second)...)
	for second.NextCursor != "" {
		second = read(url.Values{"limit": {"2"}, "cursor": {second.NextCursor}})
		if len(second.Files) > 2 {
			t.Fatal("page exceeded limit")
		}
		all = append(all, pageIDs(second)...)
	}
	if !reflect.DeepEqual(all, ids) {
		t.Fatalf("paged IDs=%v want %v", all, ids)
	}
	name := read(url.Values{"name": {"report"}, "limit": {"2"}})
	if len(name.Files) != 2 || name.NextCursor == "" {
		t.Fatalf("filtered page=%#v", name)
	}
	literal := read(url.Values{"name": {"%_"}})
	if len(literal.Files) != 1 || literal.Files[0].Name != "report%_.txt" {
		t.Fatalf("literal name filter=%#v", literal)
	}
	images := read(url.Values{"media_type": {"image/*"}, "accept": {"image/*", "text/plain"}})
	if len(images.Files) != 2 {
		t.Fatalf("media filter=%#v", images)
	}
	intersection := read(url.Values{"media_type": {"image/*"}, "accept": {"text/plain"}})
	if len(intersection.Files) != 0 {
		t.Fatalf("accept filter was widened=%#v", intersection)
	}
	selected := read(url.Values{"id": {ids[6], ids[0], "sfi_00000000000000000000000000"}})
	if !reflect.DeepEqual(pageIDs(selected), []string{ids[6], ids[0]}) {
		t.Fatalf("selected references=%#v", selected)
	}
	for _, query := range []string{"limit=100000", "limit=0", "cursor=invalid", "direction=previous", "media_type=invalid", "name=a&name=b", "unexpected=x", "id="} {
		response := inputTestRequest(fixture.handler, http.MethodGet, "/api/v1/input/field/files?"+query, token, "")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid query %s=%d %s", query, response.Code, response.Body.String())
		}
	}
}
