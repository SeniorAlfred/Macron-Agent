package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), path, Options{})
	if err != nil {
		t.Fatalf("cannot open the store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestOpenCreatesTheSchema(t *testing.T) {
	s, path := openTestStore(t)
	if DBPath(path) != path {
		t.Fatalf("DBPath changed a plain path: %q", DBPath(path))
	}
	ctx := context.Background()
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&n); err != nil {
		t.Fatalf("cannot read the schema: %v", err)
	}
	if n < 20 {
		t.Fatalf("expected the full schema, found only %d tables", n)
	}
}

func TestMigrationsAreIdempotentAndDataSurvives(t *testing.T) {
	s, path := openTestStore(t)
	ctx := context.Background()

	if _, _, err := s.TouchUser(ctx, UserInfo{ID: 7, Username: "owner", FirstName: "Sara"}); err != nil {
		t.Fatalf("cannot add the user: %v", err)
	}
	if err := s.SetSetting(ctx, "model", "gpt-5"); err != nil {
		t.Fatalf("cannot save a setting: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("cannot close: %v", err)
	}

	again, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatalf("reopening a migrated database failed: %v", err)
	}
	defer again.Close()

	u, err := again.User(ctx, 7)
	if err != nil {
		t.Fatalf("the user did not survive the reopen: %v", err)
	}
	if u.Username != "owner" || u.FirstName != "Sara" {
		t.Fatalf("wrong user after reopen: %+v", u)
	}
	if v, ok := again.Setting("model"); !ok || v != "gpt-5" {
		t.Fatalf("the setting did not survive the reopen: %q %v", v, ok)
	}
}

func TestTouchUserCountsMessages(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	u, isNew, err := s.TouchUser(ctx, UserInfo{ID: 1, FirstName: "Ali", LanguageCode: "fa"})
	if err != nil || !isNew {
		t.Fatalf("the first touch should create the user: %v (new=%v)", err, isNew)
	}
	if u.ID != 1 || u.FirstName != "Ali" {
		t.Fatalf("wrong user: %+v", u)
	}
	if _, isNew, err = s.TouchUser(ctx, UserInfo{ID: 1, FirstName: "Ali", Username: "ali"}); err != nil || isNew {
		t.Fatalf("the second touch should only update: %v (new=%v)", err, isNew)
	}
	u, err = s.User(ctx, 1)
	if err != nil {
		t.Fatalf("cannot read the user: %v", err)
	}
	if u.Username != "ali" {
		t.Fatalf("the username was not updated: %+v", u)
	}
	if err := s.CountRequest(ctx, 1); err != nil {
		t.Fatalf("cannot count a message: %v", err)
	}
	u, err = s.User(ctx, 1)
	if err != nil {
		t.Fatalf("cannot read the user: %v", err)
	}
	if u.MessagesTotal != 1 {
		t.Fatalf("expected the message to be counted, got %d", u.MessagesTotal)
	}
	if _, err := s.User(ctx, 999); err == nil {
		t.Fatal("a missing user should be an error")
	}
}

func TestSettingsFallBackToTheirDefault(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	if got := s.SettingInt("missing", 42); got != 42 {
		t.Fatalf("expected the default 42, got %d", got)
	}
	if err := s.SetSetting(ctx, "max_steps", "30"); err != nil {
		t.Fatalf("cannot save: %v", err)
	}
	if got := s.SettingInt("max_steps", 5); got != 30 {
		t.Fatalf("expected the saved 30, got %d", got)
	}
	if err := s.SetSetting(ctx, "yolo", "true"); err != nil {
		t.Fatalf("cannot save: %v", err)
	}
	if !s.SettingBool("yolo", false) {
		t.Fatal("expected the saved bool")
	}
	if s.SettingBool("missing", false) {
		t.Fatal("a missing bool must be false")
	}
	if _, ok := s.Setting("missing"); ok {
		t.Fatal("a missing setting must report false")
	}
}

func TestFactsRoundTrip(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	id, existed, err := s.AddFact(ctx, 3, "likes dark coffee")
	if err != nil || existed || id == 0 {
		t.Fatalf("cannot add a fact: %v (existed=%v id=%d)", err, existed, id)
	}
	if _, existed, err = s.AddFact(ctx, 3, "  likes dark coffee  "); err != nil || !existed {
		t.Fatalf("a repeated fact should be reported as existing: %v (existed=%v)", err, existed)
	}
	facts, err := s.Facts(ctx, 3)
	if err != nil || len(facts) != 1 {
		t.Fatalf("expected one fact, got %d (%v)", len(facts), err)
	}
	if facts[0].Text != "likes dark coffee" {
		t.Fatalf("wrong fact text: %q", facts[0].Text)
	}
	if other, _ := s.Facts(ctx, 4); len(other) != 0 {
		t.Fatalf("facts must be per user, got %d for another user", len(other))
	}
	removed, err := s.DeleteFact(ctx, 3, id)
	if err != nil || !removed {
		t.Fatalf("cannot delete the fact: %v", err)
	}
	if again, _ := s.DeleteFact(ctx, 3, id); again {
		t.Fatal("deleting it twice should report nothing removed")
	}
}

func TestCmdGateStore(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	if err := s.AddCmdAllow(ctx, "git status"); err != nil {
		t.Fatalf("cannot allow: %v", err)
	}
	if err := s.AddCmdAllow(ctx, "git status"); err != nil {
		t.Fatalf("adding it twice must be harmless: %v", err)
	}
	if err := s.AddCmdAllow(ctx, "ls *"); err != nil {
		t.Fatalf("cannot allow a pattern: %v", err)
	}
	list, err := s.CmdAllowlist(ctx)
	if err != nil {
		t.Fatalf("cannot read the allowlist: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected two patterns, got %v", list)
	}
	if err := s.RemoveCmdAllow(ctx, "git status"); err != nil {
		t.Fatalf("cannot remove: %v", err)
	}
	if list, _ = s.CmdAllowlist(ctx); len(list) != 1 || list[0] != "ls *" {
		t.Fatalf("the wrong pattern was removed: %v", list)
	}

	for _, d := range []string{"allowed", "denied"} {
		if err := s.AddCmdAudit(ctx, CmdAudit{Session: "s1", Origin: "user", Command: "ls", Decision: d, Decider: "owner", Rule: "rm-recursive"}); err != nil {
			t.Fatalf("cannot audit: %v", err)
		}
	}
	audits, err := s.CmdAudits(ctx, 10)
	if err != nil {
		t.Fatalf("cannot read the audit: %v", err)
	}
	if len(audits) != 2 {
		t.Fatalf("expected two audit entries, got %d", len(audits))
	}
	if audits[0].Decision != "denied" {
		t.Fatalf("the newest entry should come first, got %q", audits[0].Decision)
	}
	if audits[0].At.IsZero() {
		t.Fatal("the audit entry should carry a time")
	}
}

func TestUsageAndStatsDoNotCrashOnAnEmptyDatabase(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()

	if err := s.CountRequest(ctx, 5); err != nil {
		t.Fatalf("cannot count a request: %v", err)
	}
	if err := s.AddTokens(ctx, 5, 120); err != nil {
		t.Fatalf("cannot add tokens: %v", err)
	}
	n, err := s.TokensToday(ctx, 5)
	if err != nil || n != 120 {
		t.Fatalf("expected 120 tokens today, got %d (%v)", n, err)
	}
	used, err := s.UsedToday(ctx, 5)
	if err != nil || used != 1 {
		t.Fatalf("expected one request today, got %d (%v)", used, err)
	}
	if err := s.RecordStat(ctx, "tool", "web_search", false, 10, 20); err != nil {
		t.Fatalf("cannot record a stat: %v", err)
	}
	if ov, err := s.Overview(ctx); err != nil || ov == nil {
		t.Fatalf("cannot read the overview: %v", err)
	}
	if _, err := s.DailySeries(ctx, 7); err != nil {
		t.Fatalf("cannot read the daily series: %v", err)
	}
	if _, err := s.DatabaseSize(ctx); err != nil {
		t.Fatalf("cannot read the database size: %v", err)
	}
}

func TestSchedulesRoundTrip(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	when := time.Date(2026, 12, 1, 9, 30, 0, 0, time.UTC)

	id, err := s.AddSchedule(ctx, Schedule{UserID: 2, Kind: "reminder", Title: "gold", Body: "send the gold price", Rule: "daily@09:30", NextRun: when})
	if err != nil || id == 0 {
		t.Fatalf("cannot add a schedule: %v (id=%d)", err, id)
	}
	list, err := s.UserSchedules(ctx, 2)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected one schedule, got %d (%v)", len(list), err)
	}
	if list[0].Body != "send the gold price" || !list[0].NextRun.Equal(when) {
		t.Fatalf("wrong schedule: %+v", list[0])
	}
	due, err := s.DueSchedules(ctx, when.Add(time.Minute), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("expected the schedule to be due: %d (%v)", len(due), err)
	}
	if ok, err := s.CancelSchedule(ctx, id, 2); err != nil || !ok {
		t.Fatalf("cannot cancel the schedule: %v", err)
	}
	if list, _ = s.UserSchedules(ctx, 2); len(list) != 0 {
		t.Fatalf("the schedule was not cancelled: %+v", list)
	}
}
