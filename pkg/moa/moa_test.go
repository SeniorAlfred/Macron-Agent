package moa

import (
	"testing"
)

func TestParsePresets(t *testing.T) {
	if list, err := ParsePresets("   "); err != nil || list != nil {
		t.Fatalf("an empty setting should give no presets: %v %v", list, err)
	}
	raw := `[{"name":"quality","reference_models":[{"provider":"a"},{"provider":"b","reasoning_effort":"high"}],` +
		`"aggregator":{"provider":"c"},"fanout":"every_n:3"}]`
	list, err := ParsePresets(raw)
	if err != nil {
		t.Fatalf("ParsePresets: %v", err)
	}
	if len(list) != 1 || list[0].Name != "quality" || len(list[0].References) != 2 {
		t.Fatalf("wrong presets: %+v", list)
	}
	if list[0].Aggregator.Provider != "c" || list[0].References[1].Effort != "high" {
		t.Fatalf("the slots were not read: %+v", list[0])
	}
	if _, err := ParsePresets(`{"name":"x"}`); err == nil {
		t.Fatal("an object instead of a list must fail")
	}
	if _, err := ParsePresets(`not json`); err == nil {
		t.Fatal("broken JSON must fail")
	}
}

func TestPresetFanout(t *testing.T) {
	cases := []struct {
		fanout string
		every  int
		name   string
	}{
		{"", 0, FanoutUserTurn},
		{FanoutUserTurn, 0, FanoutUserTurn},
		{FanoutPerIteration, 1, FanoutPerIteration},
		{"every_n:3", 3, "every_n:3"},
		{"every_n:0", 0, FanoutUserTurn},
		{"every_n:abc", 0, FanoutUserTurn},
	}
	for _, c := range cases {
		p := Preset{Fanout: c.fanout}
		if got := p.Every(); got != c.every {
			t.Fatalf("%q: Every() = %d, want %d", c.fanout, got, c.every)
		}
		if got := p.FanoutName(); got != c.name {
			t.Fatalf("%q: FanoutName() = %q, want %q", c.fanout, got, c.name)
		}
	}
}

func TestFindAndOn(t *testing.T) {
	list := []Preset{{Name: "Quality"}, {Name: "cheap"}}
	if p, ok := Find(list, "quality"); !ok || p.Name != "Quality" {
		t.Fatalf("Find should be case-insensitive: %+v %v", p, ok)
	}
	if p, ok := Find(list, ""); !ok || p.Name != "Quality" {
		t.Fatalf("an empty name should give the first preset: %+v %v", p, ok)
	}
	if _, ok := Find(list, "missing"); ok {
		t.Fatal("a missing preset should not be found")
	}
	off := false
	if (Preset{}).On() != true {
		t.Fatal("a preset without an enabled flag is on")
	}
	if (Preset{Enabled: &off}).On() {
		t.Fatal("an explicit false must be off")
	}
}

func TestProblemsExplainsWhatIsMissing(t *testing.T) {
	known := map[string]struct {
		ok, key, moa bool
	}{
		"a": {true, true, false},
		"b": {true, false, false},
		"m": {true, true, true},
	}
	resolve := func(name string) (bool, bool, bool) {
		v, ok := known[name]
		if !ok {
			return false, false, false
		}
		return v.ok, v.key, v.moa
	}

	if got := Problems(Preset{Name: "p", Aggregator: Slot{Provider: "a"}}, resolve); len(got) != 0 {
		t.Fatalf("a healthy preset should have no problems: %v", got)
	}
	for _, c := range []struct {
		p    Preset
		want string
	}{
		{Preset{Aggregator: Slot{Provider: "a"}}, "no name"},
		{Preset{Name: "p"}, "no aggregator"},
		{Preset{Name: "p", Aggregator: Slot{Provider: "zzz"}}, "not a provider"},
		{Preset{Name: "p", Aggregator: Slot{Provider: "m"}}, "cannot be a mixture"},
		{Preset{Name: "p", Aggregator: Slot{Provider: "b"}}, "no API key"},
		{Preset{Name: "p", Aggregator: Slot{Provider: "a"}, References: []Slot{{Provider: "zzz"}}}, "not a provider"},
		{Preset{Name: "p", Aggregator: Slot{Provider: "a"}, References: []Slot{{Provider: "a", Effort: "turbo"}}}, "unknown reasoning_effort"},
	} {
		got := Problems(c.p, resolve)
		found := false
		for _, g := range got {
			if contains(g, c.want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected a problem about %q, got %v", c.want, got)
		}
	}
}

func TestScrubHidesContacts(t *testing.T) {
	in := "mail me at someone@example.com or call 0912 345 6789"
	out := Scrub(in)
	if contains(out, "someone@example.com") || contains(out, "0912 345 6789") {
		t.Fatalf("the contacts should be hidden: %q", out)
	}
	if !contains(out, "[email]") || !contains(out, "[phone]") {
		t.Fatalf("the placeholders are missing: %q", out)
	}
	if got := Scrub("no contacts here"); got != "no contacts here" {
		t.Fatalf("plain text must not change: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
