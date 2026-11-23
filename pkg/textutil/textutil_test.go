package textutil

import (
	"strings"
	"testing"
	"time"
)

func TestTruncateRunes(t *testing.T) {
	if got := TruncateRunes("سلام دنیا", 4, "…"); got != "سلام…" {
		t.Fatalf("TruncateRunes: %q", got)
	}
	if got := TruncateRunes("short", 10, "…"); got != "short" {
		t.Fatalf("a short string must not change: %q", got)
	}
	if got := TruncateRunes("anything", 0, ""); got != "" {
		t.Fatalf("a zero limit must give an empty string: %q", got)
	}
}

func TestTruncateBytesKeepsValidUTF8(t *testing.T) {
	got := TruncateBytes("سلام دنیا", 8, "…")
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected the suffix: %q", got)
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("the cut split a rune: %q", got)
	}
}

func TestCleaners(t *testing.T) {
	if got := StripHTML("<b>سلام</b> <i>دنیا</i>"); got != "سلام دنیا" {
		t.Fatalf("StripHTML: %q", got)
	}
	if got := CollapseSpace("  a \n\t b  "); got != "a b" {
		t.Fatalf("CollapseSpace: %q", got)
	}
	if got := OneLine("a\nb\r\nc"); got != "a b c" {
		t.Fatalf("OneLine: %q", got)
	}
}

func TestHeadTail(t *testing.T) {
	if got := HeadTail("short", 100); got != "short" {
		t.Fatalf("a short string must not change: %q", got)
	}
	long := strings.Repeat("a", 100) + strings.Repeat("b", 100)
	got := HeadTail(long, 60)
	if !strings.Contains(got, "omitted") {
		t.Fatalf("HeadTail should say what it dropped: %q", got)
	}
	if !strings.HasPrefix(got, "a") || !strings.HasSuffix(got, "b") {
		t.Fatalf("HeadTail should keep both ends: %q", got)
	}
	if n := len([]rune(got)); n > 130 {
		t.Fatalf("HeadTail kept too much: %d runes", n)
	}
}

func TestArgPreview(t *testing.T) {
	if got := ArgPreview(`{"query":"  hello   world "}`, 100); got != "hello world" {
		t.Fatalf("ArgPreview: %q", got)
	}
	if got := ArgPreview(`{"other":"x"}`, 100); got != "" {
		t.Fatalf("an unknown key should give nothing: %q", got)
	}
	if got := ArgPreview(`not json`, 100); got != "" {
		t.Fatalf("broken JSON should give nothing: %q", got)
	}
}

func TestFoldPersian(t *testing.T) {
	// The Arabic presentation forms must fold to the normal Persian letters.
	if got := Fold("ﻻ"); got != "لا" {
		t.Fatalf("Fold: %q", got)
	}
	if got := Fold("abc"); got != "abc" {
		t.Fatalf("Fold must leave Latin text alone: %q", got)
	}
}

func TestPersianDigits(t *testing.T) {
	if got := ToPersianDigits("2026-11-23"); got != "۲۰۲۶-۱۱-۲۳" {
		t.Fatalf("ToPersianDigits: %q", got)
	}
}

func TestJalaliConversion(t *testing.T) {
	jy, jm, jd := GregorianToJalali(2026, 11, 23)
	if jy != 1405 || jm != 9 || jd != 2 {
		t.Fatalf("GregorianToJalali(2026-11-23) = %d-%d-%d, want 1405-9-2", jy, jm, jd)
	}
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	if got := JalaliDateShort(at); !strings.Contains(got, "۱۴۰۵") && !strings.Contains(got, "1405") {
		t.Fatalf("JalaliDateShort: %q", got)
	}
	if got := JalaliDateFull(at); got == "" {
		t.Fatal("JalaliDateFull returned nothing")
	}
}

func TestPersianEzafeLeavesPlainTextAlone(t *testing.T) {
	if got := PersianEzafe("سلام"); got != "سلام" {
		t.Fatalf("PersianEzafe changed normal text: %q", got)
	}
}
