package sched

import (
	"testing"
	"time"
)

func TestParseDailyAndWeekly(t *testing.T) {
	daily, err := Parse("daily@09:30")
	if err != nil {
		t.Fatalf("daily: %v", err)
	}
	if daily.Kind != "daily" || daily.Hour != 9 || daily.Minute != 30 {
		t.Fatalf("wrong daily rule: %+v", daily)
	}

	weekly, err := Parse("weekly:mon,wed,fri@18:05")
	if err != nil {
		t.Fatalf("weekly: %v", err)
	}
	if weekly.Kind != "weekly" || weekly.Hour != 18 || weekly.Minute != 5 {
		t.Fatalf("wrong weekly rule: %+v", weekly)
	}
	if len(weekly.Days) != 3 || weekly.Days[0] != time.Monday || weekly.Days[2] != time.Friday {
		t.Fatalf("wrong weekdays: %v", weekly.Days)
	}

	monthly, err := Parse("monthly:1@00:00")
	if err != nil {
		t.Fatalf("monthly: %v", err)
	}
	if monthly.Kind != "monthly" || monthly.Day != 1 {
		t.Fatalf("wrong monthly rule: %+v", monthly)
	}

	every, err := Parse("every:2h")
	if err != nil {
		t.Fatalf("every: %v", err)
	}
	if every.Kind != "every" || every.Every != 2*time.Hour {
		t.Fatalf("wrong interval rule: %+v", every)
	}

	if empty, err := Parse("   "); err != nil || empty.Kind != "" {
		t.Fatalf("an empty rule should be empty, got %+v (%v)", empty, err)
	}
}

func TestParseRejectsNonsense(t *testing.T) {
	cases := []string{
		"every:5m",          // shorter than the minimum interval
		"every:soon",        // not a duration
		"daily@25:00",       // no such hour
		"daily@09:75",       // no such minute
		"daily",             // no time of day
		"weekly:xyz@09:00",  // no such weekday
		"weekly@09:00",      // no weekday at all
		"monthly:0@09:00",   // no such day of month
		"monthly:32@09:00",  // no such day of month
		"yearly@09:00",      // unsupported repetition
		"daily@nine thirty", // not a clock
	}
	for _, c := range cases {
		if rule, err := Parse(c); err == nil {
			t.Fatalf("Parse(%q) should fail, got %+v", c, rule)
		}
	}
}

func TestParseDayAndClock(t *testing.T) {
	if d, ok := ParseDay("دوشنبه"); !ok || d != time.Monday {
		t.Fatalf("Persian Monday: %v %v", d, ok)
	}
	if d, ok := ParseDay("saturday"); !ok || d != time.Saturday {
		t.Fatalf("English Saturday: %v %v", d, ok)
	}
	if d, ok := ParseDay("sun"); !ok || d != time.Sunday {
		t.Fatalf("short Sunday: %v %v", d, ok)
	}
	if _, ok := ParseDay("funday"); ok {
		t.Fatal("a made-up day should not parse")
	}

	if h, m, err := ParseClock("۰۹:۳۰"); err != nil || h != 9 || m != 30 {
		t.Fatalf("Persian digits: %d:%d %v", h, m, err)
	}
	if h, m, err := ParseClock(" 23:59 "); err != nil || h != 23 || m != 59 {
		t.Fatalf("plain clock: %d:%d %v", h, m, err)
	}
	for _, bad := range []string{"24:00", "12:60", "12", "12:xx", ""} {
		if _, _, err := ParseClock(bad); err == nil {
			t.Fatalf("ParseClock(%q) should fail", bad)
		}
	}
}

func TestParseLocalAcceptsTheDocumentedShapes(t *testing.T) {
	loc := time.UTC
	want := time.Date(2026, 10, 20, 11, 0, 0, 0, loc)
	for _, s := range []string{"2026-10-20 11:00", "2026-10-20T11:00", "2026/10/20 11:00"} {
		got, err := ParseLocal(s, loc)
		if err != nil {
			t.Fatalf("ParseLocal(%q): %v", s, err)
		}
		if !got.Equal(want) {
			t.Fatalf("ParseLocal(%q) = %v, want %v", s, got, want)
		}
	}
	if got, err := ParseLocal("۲۰۲۶-۱۰-۲۰ ۱۱:۰۰", loc); err != nil || !got.Equal(want) {
		t.Fatalf("Persian digits: %v %v", got, err)
	}
	if _, err := ParseLocal("tomorrow", loc); err == nil {
		t.Fatal("nonsense should not parse")
	}
}

func TestRuleStringRoundTrip(t *testing.T) {
	for _, s := range []string{"daily@09:30", "every:120m", "monthly:1@00:00"} {
		rule, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		back, err := Parse(rule.String())
		if err != nil {
			t.Fatalf("the printed rule %q does not parse: %v", rule.String(), err)
		}
		if rule.Kind != back.Kind || rule.Hour != back.Hour || rule.Minute != back.Minute || rule.Day != back.Day || rule.Every != back.Every {
			t.Fatalf("%q came back as %+v instead of %+v", s, back, rule)
		}
	}
}
