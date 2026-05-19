package opendesign

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew_UsesEnvOverride(t *testing.T) {
	t.Setenv("WUPHF_OPEN_DESIGN_URL", "http://override:9000/")
	c := New("")
	// Trailing slash trimmed so path joins don't produce double slashes.
	if c.BaseURL != "http://override:9000" {
		t.Errorf("BaseURL = %q, want http://override:9000", c.BaseURL)
	}
}

func TestNew_ExplicitWinsOverEnv(t *testing.T) {
	t.Setenv("WUPHF_OPEN_DESIGN_URL", "http://env:9000")
	c := New("http://explicit:1234")
	if c.BaseURL != "http://explicit:1234" {
		t.Errorf("BaseURL = %q, want explicit override", c.BaseURL)
	}
}

func TestNew_DefaultWhenNothingSet(t *testing.T) {
	t.Setenv("WUPHF_OPEN_DESIGN_URL", "")
	c := New("")
	if c.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want default %q", c.BaseURL, DefaultBaseURL)
	}
}

func TestHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("path = %q, want /api/health", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestHealth_DaemonDown(t *testing.T) {
	// 127.0.0.1:1 is a port no daemon will be on — connect refused.
	c := New("http://127.0.0.1:1")
	err := c.Health(context.Background())
	if err == nil {
		t.Fatal("Health: want error, got nil")
	}
	if !errors.Is(err, ErrDaemonUnreachable) {
		t.Errorf("err = %v, want wrapped ErrDaemonUnreachable", err)
	}
}

func TestHealth_Non200IsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Health(context.Background())
	if !errors.Is(err, ErrDaemonUnreachable) {
		t.Errorf("err = %v, want wrapped ErrDaemonUnreachable on 5xx", err)
	}
}

func TestListSkills_ForwardsScenarioFilter(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skills": []Skill{{ID: "guizang-ppt", Name: "Deck", Scenario: "design", Mode: "deck"}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	skills, err := c.ListSkills(context.Background(), "design")
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if gotQuery != "scenario=design" {
		t.Errorf("query = %q, want scenario=design", gotQuery)
	}
	if len(skills) != 1 || skills[0].ID != "guizang-ppt" {
		t.Errorf("skills = %+v, want one skill with ID guizang-ppt", skills)
	}
}

func TestCreateArtifact_PostsPayload(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody CreateArtifactInput
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(LiveArtifact{ID: "art-1", Status: "ready"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	art, err := c.CreateArtifact(context.Background(), CreateArtifactInput{
		Prompt: "magazine cover for seed round",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api/tools/live-artifacts/create" {
		t.Errorf("path = %q, want /api/tools/live-artifacts/create", gotPath)
	}
	if gotBody.Prompt != "magazine cover for seed round" {
		t.Errorf("body.Prompt = %q, want forwarded prompt", gotBody.Prompt)
	}
	if art.ID != "art-1" {
		t.Errorf("art.ID = %q, want art-1", art.ID)
	}
}

func TestPreviewURL_PathEscaped(t *testing.T) {
	c := New("http://daemon:7878")
	// IDs with slashes / spaces are unlikely but must be safe; the
	// escape ensures the path component round-trips through net/url.
	got := c.PreviewURL("art/with space")
	if !strings.HasPrefix(got, "http://daemon:7878/api/live-artifacts/") {
		t.Errorf("preview URL %q has unexpected prefix", got)
	}
	if strings.Contains(got, " ") {
		t.Errorf("preview URL %q contains raw space (not escaped)", got)
	}
}
