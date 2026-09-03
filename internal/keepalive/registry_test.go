package keepalive

import (
	"strings"
	"testing"
)

const (
	urlOne = "postgresql://postgres.aaa:pw1@aws-0-us-east-1.pooler.supabase.com:5432/postgres"
	urlTwo = "postgresql://postgres.bbb:pw2@aws-0-eu-west-2.pooler.supabase.com:5432/postgres"
)

func TestBuildProjectsBroadcastsASingleTable(t *testing.T) {
	projects, err := BuildProjects([]string{urlOne, urlTwo}, []string{"public.ping"})
	if err != nil {
		t.Fatalf("BuildProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
	for _, project := range projects {
		if project.Table != "public.ping" {
			t.Errorf("table = %q, want public.ping", project.Table)
		}
	}
	if projects[0].ID != "p1" || projects[1].ID != "p2" {
		t.Errorf("ids = %q/%q, want p1/p2", projects[0].ID, projects[1].ID)
	}
}

func TestBuildProjectsAlignsTablesByPosition(t *testing.T) {
	projects, err := BuildProjects([]string{urlOne, urlTwo}, []string{"public.a", "public.b"})
	if err != nil {
		t.Fatalf("BuildProjects: %v", err)
	}
	if projects[0].Table != "public.a" || projects[1].Table != "public.b" {
		t.Errorf("tables = %q/%q", projects[0].Table, projects[1].Table)
	}
	if projects[0].Quoted != `"public"."a"` {
		t.Errorf("quoted = %q", projects[0].Quoted)
	}
}

func TestBuildProjectsRejectsMismatchedCounts(t *testing.T) {
	_, err := BuildProjects([]string{urlOne, urlTwo}, []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "3 entries") || !strings.Contains(err.Error(), "2") {
		t.Errorf("error %q should name both counts", err)
	}
}

func TestBuildProjectsRequiresURLsAndTables(t *testing.T) {
	if _, err := BuildProjects(nil, []string{"public.ping"}); err == nil ||
		!strings.Contains(err.Error(), "SUPABASE_URLS is empty") {
		t.Errorf("err = %v, want SUPABASE_URLS is empty", err)
	}
	if _, err := BuildProjects([]string{urlOne}, nil); err == nil ||
		!strings.Contains(err.Error(), "SUPABASE_TABLES is empty") {
		t.Errorf("err = %v, want SUPABASE_TABLES is empty", err)
	}
}

func TestBuildProjectsNamesBadEntryWithoutLeakingIt(t *testing.T) {
	_, err := BuildProjects(
		[]string{urlOne, "postgresql://nopassword@host.example.com/db"}, []string{"public.ping"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "entry #2") {
		t.Errorf("error %q should name entry #2", err)
	}
	if strings.Contains(err.Error(), "nopassword") {
		t.Errorf("error %q must not echo the connection string", err)
	}
}

func TestBuildProjectsRejectsBadTableName(t *testing.T) {
	_, err := BuildProjects([]string{urlOne}, []string{"users; drop table x"})
	if err == nil || !strings.Contains(err.Error(), "SUPABASE_TABLES entry for project #1") {
		t.Errorf("err = %v", err)
	}
}
