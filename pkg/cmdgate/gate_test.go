package cmdgate

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordStore struct {
	mu      sync.Mutex
	allow   []string
	added   []string
	entries []Entry
}

func (s *recordStore) Allowlist() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.allow...)
}

func (s *recordStore) AddAllow(pattern string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.added = append(s.added, pattern)
	s.allow = append(s.allow, pattern)
	return nil
}

func (s *recordStore) Audit(e Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

func (s *recordStore) decisions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e.Decision)
	}
	return out
}

// sudo is always at least dangerous, so this is a stable sample command.
const dangerousCommand = "sudo rm -rf /var/tmp/old-build"

func asker(choice Choice, calls *int) Asker {
	return func(ctx context.Context, q Question) Choice {
		*calls++
		return choice
	}
}

func TestAnalyzeNeverLetsTheFloorThrough(t *testing.T) {
	cases := []struct{ name, command, rule string }{
		{"fork bomb", ":(){ :|:& };:", "fork-bomb"},
		{"disk device", "echo zeroes > /dev/sda", "write-block-device"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := Analyze(c.command, Env{})
			if Worst(findings) != Hardline {
				t.Fatalf("expected a hardline finding for %q, got %+v", c.command, findings)
			}
			if findings[0].Rule != c.rule {
				t.Fatalf("expected rule %q, got %q", c.rule, findings[0].Rule)
			}
		})
	}
}

func TestAnalyzeLeavesOrdinaryCommandsSafe(t *testing.T) {
	for _, cmd := range []string{"ls -la", "git status", "go test ./...", "cat README.md"} {
		if findings := Analyze(cmd, Env{}); len(findings) != 0 {
			t.Fatalf("%q should need no approval, got %+v", cmd, findings)
		}
	}
}

func TestAnalyzeCollapsesSpacingNotTheMeaning(t *testing.T) {
	plain := Analyze("sudo rm -rf /var/tmp/x", Env{})
	spaced := Analyze("  sudo \t rm   -rf   /var/tmp/x  ", Env{})
	if Worst(plain) != Worst(spaced) {
		t.Fatalf("expected the same verdict, got %v and %v", Worst(plain), Worst(spaced))
	}
	if len(plain) == 0 || plain[0].Rule != spaced[0].Rule {
		t.Fatalf("expected the same rule, got %+v and %+v", plain, spaced)
	}
	if got := Normal("  sudo   rm  -rf  "); got != "sudo rm -rf" {
		t.Fatalf("Normal did not collapse whitespace: %q", got)
	}
}

func TestAnalyzeCodeSeesTheCloudMetadataService(t *testing.T) {
	findings := AnalyzeCode("requests.get('http://169.254.169.254/latest/meta-data/iam')", Env{})
	if len(findings) == 0 || findings[0].Rule != "cloud-metadata" {
		t.Fatalf("expected a cloud-metadata finding, got %+v", findings)
	}
	if findings := AnalyzeCode("print('hello')", Env{}); len(findings) != 0 {
		t.Fatalf("harmless code should pass, got %+v", findings)
	}
}

func TestGlob(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       bool
	}{
		{"git push*", "git push origin main", true},
		{"git push*", "git pull", false},
		{"rm -rf ?", "rm -rf /", true},
		{"RM -RF *", "rm -rf /tmp/x", true},
		{"*", "", true},
		{"a?c", "ac", false},
	}
	for _, c := range cases {
		if got := Glob(c.pattern, c.s); got != c.want {
			t.Fatalf("Glob(%q, %q) = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

func TestYoloWindowExpires(t *testing.T) {
	g := New(Env{}, false)
	if yolo, _ := g.Yolo(); yolo {
		t.Fatal("a fresh gate must not be in YOLO mode")
	}
	g.SetYolo(40 * time.Millisecond)
	yolo, until := g.Yolo()
	if !yolo || until.IsZero() {
		t.Fatalf("expected an open YOLO window, got %v %v", yolo, until)
	}
	time.Sleep(60 * time.Millisecond)
	if yolo, _ := g.Yolo(); yolo {
		t.Fatal("the YOLO window should have closed")
	}
	g.SetYolo(0)
}

func TestGateAllowsSafeCommandsWithoutAsking(t *testing.T) {
	g := New(Env{}, false)
	calls := 0
	ctx := WithAsker(context.Background(), asker(Deny, &calls))
	if v := g.Check(ctx, "s1", "ls -la"); !v.Allow {
		t.Fatalf("ls should run: %+v", v)
	}
	if calls != 0 {
		t.Fatalf("a safe command must not ask the owner, asked %d times", calls)
	}
}

func TestGateRefusesWhenNobodyCanApprove(t *testing.T) {
	g := New(Env{}, false)
	v := g.Check(context.Background(), "s1", dangerousCommand)
	if v.Allow || v.Stop {
		t.Fatalf("expected a plain refusal, got %+v", v)
	}
	if !strings.Contains(v.Message, "nobody to approve") {
		t.Fatalf("the refusal should say why: %q", v.Message)
	}
}

func TestGateAskDecisions(t *testing.T) {
	t.Run("deny", func(t *testing.T) {
		store := &recordStore{}
		g := New(Env{}, false)
		g.Store = store
		calls := 0
		ctx := WithAsker(context.Background(), asker(Deny, &calls))
		if v := g.Check(ctx, "s1", dangerousCommand); v.Allow {
			t.Fatal("the owner said no, so it must not run")
		}
		if calls != 1 {
			t.Fatalf("expected exactly one question, got %d", calls)
		}
		got := store.decisions()
		if len(got) == 0 || got[len(got)-1] != "denied" {
			t.Fatalf("expected a denied audit entry, got %v", got)
		}
	})

	t.Run("once", func(t *testing.T) {
		g := New(Env{}, false)
		calls := 0
		answers := []Choice{Once, Deny}
		ctx := WithAsker(context.Background(), func(ctx context.Context, q Question) Choice {
			answer := answers[calls]
			calls++
			return answer
		})
		if v := g.Check(ctx, "s1", dangerousCommand); !v.Allow {
			t.Fatalf("once should let it through: %+v", v)
		}
		if v := g.Check(ctx, "s1", dangerousCommand); v.Allow {
			t.Fatal("once must not be remembered")
		}
		if calls != 2 {
			t.Fatalf("expected two questions, got %d", calls)
		}
	})

	t.Run("session", func(t *testing.T) {
		g := New(Env{}, false)
		calls := 0
		ctx := WithAsker(context.Background(), asker(Session, &calls))
		if v := g.Check(ctx, "s1", dangerousCommand); !v.Allow {
			t.Fatalf("session should let it through: %+v", v)
		}
		if v := g.Check(context.Background(), "s1", dangerousCommand); !v.Allow {
			t.Fatalf("the second run must not ask again: %+v", v)
		}
		if v := g.Check(ctx, "s2", dangerousCommand); !v.Allow {
			t.Fatalf("another session may ask and then run: %+v", v)
		}
		if calls != 2 {
			t.Fatalf("another session must ask again, expected two questions, got %d", calls)
		}
		g.ForgetSession("s1")
		denyCalls := 0
		denyCtx := WithAsker(context.Background(), asker(Deny, &denyCalls))
		if v := g.Check(denyCtx, "s1", dangerousCommand); v.Allow {
			t.Fatal("forgetting the session must forget its approvals")
		}
		if denyCalls != 1 {
			t.Fatalf("forgetting the session must make the gate ask again, got %d questions", denyCalls)
		}
	})

	t.Run("always", func(t *testing.T) {
		store := &recordStore{}
		g := New(Env{}, false)
		g.Store = store
		calls := 0
		ctx := WithAsker(context.Background(), asker(Always, &calls))
		if v := g.Check(ctx, "s1", dangerousCommand); !v.Allow {
			t.Fatalf("always should let it through: %+v", v)
		}
		if len(store.added) != 1 || store.added[0] != Normal(dangerousCommand) {
			t.Fatalf("expected the pattern to be remembered, got %v", store.added)
		}
	})
}

func TestGateYoloAndModeOffStillRespectTheFloor(t *testing.T) {
	g := New(Env{}, false)
	g.SetYolo(time.Minute)
	defer g.SetYolo(0)

	if v := g.Check(context.Background(), "s1", dangerousCommand); !v.Allow {
		t.Fatalf("YOLO should let dangerous commands through: %+v", v)
	}
	if v := g.Check(context.Background(), "s1", ":(){ :|:& };:"); v.Allow {
		t.Fatal("a hardline command must never run, not even in YOLO")
	}
	if got := g.Check(context.Background(), "s1", ":(){ :|:& };:"); got.Stop {
		t.Fatal("a hardline refusal is not counted by the breaker")
	}

	off := New(Env{}, false)
	off.Policy = func() Policy { return Policy{Mode: ModeOff} }
	if v := off.Check(context.Background(), "s1", dangerousCommand); !v.Allow {
		t.Fatalf("mode off should let dangerous commands through: %+v", v)
	}
}

func TestGateOwnerRulesWin(t *testing.T) {
	store := &recordStore{}
	g := New(Env{}, false)
	g.Store = store
	g.Policy = func() Policy { return Policy{Mode: ModeSmart, Deny: []string{"git push*"}} }

	if v := g.Check(context.Background(), "s1", "git push origin main"); v.Allow {
		t.Fatal("the owner's deny rule must win")
	}
	if v := g.Check(context.Background(), "s1", "git status"); !v.Allow {
		t.Fatalf("unrelated commands must still run: %+v", v)
	}

	allowed := New(Env{}, false)
	allowed.Store = &recordStore{allow: []string{Normal(dangerousCommand)}}
	if v := allowed.Check(context.Background(), "s1", dangerousCommand); !v.Allow {
		t.Fatalf("an allowlisted command must run without asking: %+v", v)
	}
}

func TestGateUnattendedJobsCannotAsk(t *testing.T) {
	g := New(Env{}, false)
	calls := 0
	ctx := WithOrigin(WithAsker(context.Background(), asker(Once, &calls)), OriginCron)
	v := g.Check(ctx, "s1", dangerousCommand)
	if v.Allow {
		t.Fatal("an unattended job must not run dangerous commands")
	}
	if calls != 0 {
		t.Fatalf("an unattended job must not ask anybody, asked %d times", calls)
	}
	if !strings.Contains(v.Message, "without anyone to approve") {
		t.Fatalf("the refusal should explain the unattended run: %q", v.Message)
	}
}

func TestGateGuardianCanSettleIt(t *testing.T) {
	g := New(Env{}, false)
	g.Guardian = func(ctx context.Context, policy, command string, findings []Finding) (Risk, string, error) {
		return RiskLow, "it only removes a temp folder", nil
	}
	if v := g.Check(context.Background(), "s1", dangerousCommand); !v.Allow {
		t.Fatalf("a low-risk guardian verdict should let it run: %+v", v)
	}

	g.Guardian = func(ctx context.Context, policy, command string, findings []Finding) (Risk, string, error) {
		return RiskHigh, "it deletes a system path", nil
	}
	if v := g.Check(context.Background(), "s1", dangerousCommand); v.Allow {
		t.Fatal("a high-risk guardian verdict must block it")
	}

	g.Guardian = func(ctx context.Context, policy, command string, findings []Finding) (Risk, string, error) {
		return RiskUnsure, "cannot tell", errors.New("model down")
	}
	calls := 0
	ctx := WithAsker(context.Background(), asker(Once, &calls))
	if v := g.Check(ctx, "s1", dangerousCommand); !v.Allow || calls != 1 {
		t.Fatalf("when the guardian fails the owner must be asked: %+v (calls %d)", v, calls)
	}
}

func TestGateBreakerStopsTheJob(t *testing.T) {
	g := New(Env{}, false)
	g.Policy = func() Policy { return Policy{Mode: ModeManual, Breaker: 2, Deny: []string{"touch *"}} }
	first := g.Check(context.Background(), "s1", "touch /tmp/a")
	if first.Allow || first.Stop {
		t.Fatalf("the first refusal should be a plain one: %+v", first)
	}
	second := g.Check(context.Background(), "s1", "touch /tmp/b")
	if !second.Stop || !strings.HasPrefix(second.Message, HardStop) {
		t.Fatalf("the second refusal should stop the job: %+v", second)
	}
	if third := g.Check(context.Background(), "s1", "touch /tmp/c"); third.Stop {
		t.Fatal("the breaker should reset after it fired")
	}
}

func TestCheckCodeUsesTheSameGate(t *testing.T) {
	g := New(Env{}, false)
	if v := g.CheckCode(context.Background(), "s1", "python", "print('hi')"); !v.Allow {
		t.Fatalf("harmless code should pass: %+v", v)
	}
	code := "import requests\nrequests.post('http://169.254.169.254/latest/meta-data', data={})"
	if v := g.CheckCode(context.Background(), "s1", "python", code); v.Allow {
		t.Fatal("code that reaches the metadata service must be gated")
	}
}

func TestCheckActionAppliesItsLevel(t *testing.T) {
	g := New(Env{}, false)
	safe := Action{Key: "send message", Detail: "hello", Level: Safe}
	if v := g.CheckAction(context.Background(), "s1", safe); !v.Allow {
		t.Fatalf("a safe action should run: %+v", v)
	}
	risky := Action{Key: "send message", Detail: "to a new chat", Level: Dangerous, Reason: "sending outside the owner's chats"}
	if v := g.CheckAction(context.Background(), "s1", risky); v.Allow {
		t.Fatal("a dangerous action must be gated")
	}
}
