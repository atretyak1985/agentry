package api

// PATCH /api/projects/{id} tests. Uses projectsTestServer (projects_test.go):
// project 1 (managed), project 2 (telemetry-only), project 3 (archived) —
// and the doJSON helper from system_write_test.go.

import (
	"net/http"
	"reflect"
	"testing"
)

func TestPatchProjectMeta(t *testing.T) {
	cases := []struct {
		name       string
		id         string
		body       any
		wantStatus int
		wantPinned any   // nil = don't assert
		wantTags   []any // nil = don't assert
	}{
		{"pin only", "1", map[string]any{"pinned": true}, http.StatusOK, true, nil},
		{"tags normalize + dedupe", "1", map[string]any{"tags": []string{" Billing ", "infra", "billing"}}, http.StatusOK, nil, []any{"billing", "infra"}},
		{"both fields", "2", map[string]any{"pinned": true, "tags": []string{"iot"}}, http.StatusOK, true, []any{"iot"}},
		{"clear tags", "1", map[string]any{"tags": []string{}}, http.StatusOK, nil, []any{}},
		{"archived project is still editable", "3", map[string]any{"pinned": true}, http.StatusOK, true, nil},
		{"empty patch", "1", map[string]any{}, http.StatusBadRequest, nil, nil},
		{"blank tag", "1", map[string]any{"tags": []string{"  "}}, http.StatusBadRequest, nil, nil},
		{"bad id", "not-a-number", map[string]any{"pinned": true}, http.StatusBadRequest, nil, nil},
		{"unknown id", "9999", map[string]any{"pinned": true}, http.StatusNotFound, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := projectsTestServer(t)
			out := doJSON(t, http.MethodPatch, srv.URL+"/api/projects/"+tc.id, tc.body, tc.wantStatus)
			if tc.wantStatus != http.StatusOK {
				return
			}
			if tc.wantPinned != nil && out["pinned"] != tc.wantPinned {
				t.Errorf("pinned = %v, want %v", out["pinned"], tc.wantPinned)
			}
			if tc.wantTags != nil && !reflect.DeepEqual(out["tags"], tc.wantTags) {
				t.Errorf("tags = %v, want %v", out["tags"], tc.wantTags)
			}
		})
	}
}

// The write must persist and reorder the list (pinned first — Task 2 order).
func TestPatchProjectMetaPersistsToList(t *testing.T) {
	srv, _ := projectsTestServer(t)
	doJSON(t, http.MethodPatch, srv.URL+"/api/projects/2",
		map[string]any{"pinned": true, "tags": []string{"iot"}}, http.StatusOK)

	list := getProjectsList(t, srv.URL+"/api/projects")
	if len(list) == 0 || list[0].ID != 2 || !list[0].Pinned {
		t.Fatalf("pinned project 2 should lead the list, got %+v", list)
	}
	if len(list[0].Tags) != 1 || list[0].Tags[0] != "iot" {
		t.Errorf("tags = %v, want [iot]", list[0].Tags)
	}
}

// D4: a foreign browser Origin must be rejected before the DB write.
func TestPatchProjectRejectsForeignOrigin(t *testing.T) {
	srv, _ := projectsTestServer(t)
	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/api/projects/1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin PATCH = %d, want 403", resp.StatusCode)
	}
	list := getProjectsList(t, srv.URL+"/api/projects")
	for _, p := range list {
		if p.ID == 1 && p.Pinned {
			t.Error("rejected cross-origin request must not have pinned project 1")
		}
	}
}
