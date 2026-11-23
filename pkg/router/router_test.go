package router

import "testing"

func TestClassifyFindsTheMainKinds(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want Kind
	}{
		{"greeting", Input{Text: "سلام، خوبی؟"}, Chat},
		{"price", Input{Text: "قیمت دلار امروز چنده؟"}, Quick},
		{"code", Input{Text: "این تابع پایتون رو دیباگ کن\n```python\ndef f(): pass\n```"}, Code},
		{"research", Input{Text: "یک بررسی جامع با منابع معتبر درباره‌ی رمزارزها بنویس"}, Research},
		{"documents", Input{Text: "این فایل اکسل رو تحلیل کن و گزارش مالی بده"}, Documents},
		{"media", Input{Text: "این ویدیو یوتیوب رو دانلود کن"}, Media},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := Classify(c.in)
			if v.Kind != c.want {
				t.Fatalf("Classify(%q) = %q (%v), want %q", c.in.Text, v.Kind, v.Signals, c.want)
			}
			if len(v.Scores) == 0 {
				t.Fatal("a verdict should carry its scores")
			}
		})
	}
}

func TestClassifyImageInputIsVision(t *testing.T) {
	v := Classify(Input{Text: "این چیه؟", Images: 1})
	if v.Kind != Vision {
		t.Fatalf("an image should be vision, got %q (%v)", v.Kind, v.Signals)
	}
}

func TestDecideFollowsTheSensitivity(t *testing.T) {
	strong := map[Kind]bool{Code: true}
	if !Decide(Verdict{Kind: Code}, strong, Cheap) {
		t.Fatal("a strong kind should always get the strong model")
	}
	if Decide(Verdict{Kind: Chat}, strong, Cheap) {
		t.Fatal("a cheap run should keep chat on the cheap model")
	}
	if !Decide(Verdict{Kind: Chat, Hard: true}, strong, Balanced) {
		t.Fatal("a hard question gets the strong model when balanced")
	}
	if Decide(Verdict{Kind: Chat, Hard: true}, strong, Cheap) {
		t.Fatal("a cheap run ignores hardness")
	}
	if !Decide(Verdict{Kind: Documents}, strong, Quality) {
		t.Fatal("quality sends documents to the strong model")
	}
	if Decide(Verdict{Kind: Media}, strong, Quality) {
		t.Fatal("quality still keeps media cheap")
	}
}

func TestValidKind(t *testing.T) {
	for _, k := range Kinds {
		if !ValidKind(k) {
			t.Fatalf("%q should be valid", k)
		}
	}
	if ValidKind("nonsense") {
		t.Fatal("an invented kind must not be valid")
	}
}
