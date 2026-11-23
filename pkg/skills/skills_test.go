package skills

import (
	"errors"
	"strings"
	"testing"
)

func TestValidNameAndNormalize(t *testing.T) {
	for _, ok := range []string{"code-review", "linux2", "a.b_c", "ab"} {
		if !ValidName(ok) {
			t.Fatalf("%q should be a valid name", ok)
		}
	}
	for _, bad := range []string{"", "a", "Upper", "has space", "-leading", strings.Repeat("a", 70)} {
		if ValidName(bad) {
			t.Fatalf("%q should not be a valid name", bad)
		}
	}
	if got := NormalizeName("  Code Review  "); got != "code-review" {
		t.Fatalf("NormalizeName: %q", got)
	}
}

func TestParseAndRenderFrontMatter(t *testing.T) {
	raw := "---\nname: code-review\ndescription: \"Use when reviewing: code\"\ncategory: dev\n---\n\n# Review\n\nBody text.\n"
	meta, body := Parse(raw)
	if meta.get("name") != "code-review" {
		t.Fatalf("wrong name: %q", meta.get("name"))
	}
	if meta.get("description") != "Use when reviewing: code" {
		t.Fatalf("wrong description: %q", meta.get("description"))
	}
	if body != "# Review\n\nBody text." {
		t.Fatalf("wrong body: %q", body)
	}

	again, bodyAgain := Parse(Render(meta, body))
	if bodyAgain != body {
		t.Fatalf("the body did not survive the round trip: %q", bodyAgain)
	}
	for _, key := range []string{"name", "description", "category"} {
		if again.get(key) != meta.get(key) {
			t.Fatalf("%s did not survive the round trip: %q != %q", key, again.get(key), meta.get(key))
		}
	}

	if meta, body := Parse("no front matter here"); meta != nil || body != "no front matter here" {
		t.Fatalf("plain markdown should pass through: %v %q", meta, body)
	}
}

func TestStoreLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.Len() != 0 {
		t.Fatalf("a fresh folder should hold no skills, found %d", s.Len())
	}

	if err := s.Save("code-review", "Use when reviewing code", "# Review\n\nRead the diff first.", "dev"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	sk, ok := s.Get("code-review")
	if !ok {
		t.Fatal("the saved skill is missing")
	}
	if sk.Category != "dev" || !sk.Enabled || sk.Version != DefaultVersion {
		t.Fatalf("wrong skill: %+v", sk)
	}
	if raw, err := s.ReadFile("code-review", FileName); err != nil || !strings.Contains(raw, "Read the diff first.") {
		t.Fatalf("cannot read the saved file: %q (%v)", raw, err)
	}
	if raw, err := s.ReadFile("code-review", "../escape"); err == nil {
		t.Fatalf("a path outside the skill folder must be refused: %q", raw)
	}

	if err := s.SetEnabled("code-review", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if sk, _ := s.Get("code-review"); sk.Enabled {
		t.Fatal("the skill should be off")
	}
	reopened, err := New(dir)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if sk, ok := reopened.Get("code-review"); !ok || sk.Enabled {
		t.Fatalf("the disabled skill did not survive the reopen: %+v", sk)
	}

	if err := reopened.Delete("code-review"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := reopened.Get("code-review"); ok {
		t.Fatal("the skill should be gone")
	}
	if err := reopened.Delete("code-review"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting it twice should report ErrNotFound, got %v", err)
	}
}

func TestSaveRejectsBadInput(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := []struct {
		name, description, body, category string
		want                              error
	}{
		{"a", "d", "b", "", ErrBadName},
		{"good-name", "", "b", "", ErrNoDescription},
		{"good-name", strings.Repeat("x", MaxDescription+1), "b", "", ErrLongDescription},
		{"good-name", "d", "   ", "", ErrEmptyBody},
		{"good-name", "d", "b", "x", ErrBadCategory},
	}
	for _, c := range cases {
		err := s.Save(c.name, c.description, c.body, c.category)
		if !errors.Is(err, c.want) {
			t.Fatalf("Save(%q, %q, %q, %q) = %v, want %v", c.name, c.description, c.body, c.category, err, c.want)
		}
	}
}

func TestSeedInstallsTheBundledSkills(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	added, err := s.Seed()
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if len(added) == 0 {
		t.Fatal("expected the bundled skills to be added")
	}
	if s.Len() != len(added) {
		t.Fatalf("expected %d skills, found %d", len(added), s.Len())
	}
	if _, ok := s.Get("code-review"); !ok {
		t.Fatal("the code-review seed is missing")
	}
	again, err := s.Seed()
	if err != nil {
		t.Fatalf("a second Seed: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("seeding twice must be a no-op, added %v", again)
	}
	if index := s.Index(); !strings.Contains(index, "code-review") {
		t.Fatalf("the index should list the skills: %q", index)
	}
}
