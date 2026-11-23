package messenger

import (
	"strings"
	"testing"
)

func TestNewAcceptsTheSupportedPlatforms(t *testing.T) {
	for _, platform := range []string{"bale", "rubika"} {
		c, err := New(platform, "123:abc")
		if err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		if !strings.HasSuffix(c.BaseURL, "123:abc") {
			t.Fatalf("%s: the token should end the base URL, got %q", platform, c.BaseURL)
		}
		if c.HTTP == nil || c.HTTP.Timeout == 0 {
			t.Fatalf("%s: the client needs a timeout", platform)
		}
		if !strings.Contains(c.CursorKey(), platform) {
			t.Fatalf("%s: the cursor key should name the platform: %q", platform, c.CursorKey())
		}
	}
}

func TestNewRejectsUnknownPlatformsAndEmptyTokens(t *testing.T) {
	if _, err := New("telegram", "1:x"); err == nil {
		t.Fatal("telegram has its own bot, the inbound runner must refuse it")
	}
	if _, err := New("eitaa", "1:x"); err == nil {
		t.Fatal("eitaa is outbound only, the inbound runner must refuse it")
	}
	if _, err := New("bale", "   "); err == nil {
		t.Fatal("an empty token must be refused")
	}
}

func TestCursorKeyHidesTheToken(t *testing.T) {
	a, _ := New("bale", "secret-one")
	b, _ := New("bale", "secret-two")
	if strings.Contains(a.CursorKey(), "secret-one") {
		t.Fatalf("the cursor key must not contain the token: %q", a.CursorKey())
	}
	if a.CursorKey() == b.CursorKey() {
		t.Fatal("different tokens must give different cursors")
	}
	if a.CursorKey() != a.CursorKey() {
		t.Fatal("the cursor key must be stable")
	}
}

func TestSetAPIBaseValidatesTheAddress(t *testing.T) {
	c, err := New("bale", "t")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.SetAPIBase("http://127.0.0.1:8080"); err != nil {
		t.Fatalf("a local fixture should be accepted: %v", err)
	}
	if !strings.HasSuffix(c.BaseURL, "/bot"+"t") {
		t.Fatalf("bale should keep its /bot path: %q", c.BaseURL)
	}
	for _, bad := range []string{"127.0.0.1:8080", "ftp://host", "http://user:pass@host", "http://host?x=1"} {
		if err := c.SetAPIBase(bad); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
}

func TestUserIDIsStableAndNegative(t *testing.T) {
	a := UserID("bale", "42")
	if a != UserID("bale", "42") {
		t.Fatal("the same user must get the same id")
	}
	if a >= 0 {
		t.Fatalf("messenger users must not collide with Telegram ids, got %d", a)
	}
	if a == UserID("rubika", "42") {
		t.Fatalf("the platform must be part of the id: %d", a)
	}
	if a == UserID("bale", "43") {
		t.Fatal("different users must get different ids")
	}
}
