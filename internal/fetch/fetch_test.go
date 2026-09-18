package fetch

import (
	"strings"
	"testing"
)

func TestNormalizeYouTubeShorts(t *testing.T) {
	got, err := NormalizeURL("https://www.youtube.com/shorts/abc123XYZ_?si=track")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://www.youtube.com/watch?v=abc123XYZ_" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeYoutuBe(t *testing.T) {
	got, err := NormalizeURL("https://youtu.be/abc123XYZ_?si=junk")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://www.youtube.com/watch?v=abc123XYZ_" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeXToTwitter(t *testing.T) {
	got, err := NormalizeURL("https://x.com/user/status/123")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "twitter.com") {
		t.Fatalf("got %q", got)
	}
}

func TestSnapHeight(t *testing.T) {
	cases := map[int]int{0: 4320, 500: 480, 720: 720, 900: 720, 1080: 1080, 9999: 4320}
	for in, want := range cases {
		if got := SnapHeight(in); got != want {
			t.Fatalf("SnapHeight(%d)=%d want %d", in, got, want)
		}
	}
}

func TestClassifyTikTok(t *testing.T) {
	c := Classify("TikTok blocked extraction (no page data)", "tiktok")
	if c.Code != CodeTikTokBlocked {
		t.Fatalf("code %s", c.Code)
	}
	if !c.Retry {
		t.Fatal("expected retry")
	}
}

func TestClassifyPrivate(t *testing.T) {
	c := Classify("ERROR: Private video", "youtube")
	if c.Code != CodePrivate || c.Retry {
		t.Fatalf("%+v", c)
	}
}

func TestShouldRetry(t *testing.T) {
	ok := Classified{Retry: true}
	if !ShouldRetryAttempt(0, 3, ok) {
		t.Fatal("expected retry")
	}
	if ShouldRetryAttempt(2, 3, ok) {
		t.Fatal("last attempt should not retry")
	}
}

func TestBackoffGrows(t *testing.T) {
	a := BackoffMs(0, 1)
	b := BackoffMs(2, 1)
	if b <= a {
		t.Fatalf("expected growth %d vs %d", a, b)
	}
}

func TestMaxAttemptsTikTok(t *testing.T) {
	if MaxAttemptsFor("tiktok") <= DefaultMaxAttempts {
		t.Fatal("tiktok should retry more")
	}
}

func TestTwitterTokenNonEmpty(t *testing.T) {
	tok := twitterSyndicationToken("7649035833973984532")
	if tok == "" {
		t.Fatal("empty token")
	}
}

func TestExtractTikTokID(t *testing.T) {
	u := mustURL("https://www.tiktok.com/@oun.ra3290/video/7649035833973984532")
	if id := extractTikTokID(u); id != "7649035833973984532" {
		t.Fatalf("id=%q", id)
	}
}

func TestExtractTikTokPhotoID(t *testing.T) {
	u := mustURL("https://www.tiktok.com/@yuvin_la/photo/7648556114463722773")
	if id := extractTikTokID(u); id != "7648556114463722773" {
		t.Fatalf("id=%q", id)
	}
}

func TestExtractTikTokShort(t *testing.T) {
	u := mustURL("https://vt.tiktok.com/ZSVxYeXad")
	if s := extractTikTokShort(u); s != "ZSVxYeXad" {
		t.Fatalf("short=%q", s)
	}
}

func TestFirstTikTokImage(t *testing.T) {
	detail := map[string]any{
		"imagePost": map[string]any{
			"images": []any{
				map[string]any{
					"imageURL": map[string]any{
						"urlList": []any{
							"https://cdn.example/img.jpeg?x=1",
							"https://cdn.example/img.webp",
						},
					},
				},
			},
		},
	}
	got := firstTikTokImage(detail)
	if !strings.Contains(got, ".jpeg") {
		t.Fatalf("got %q", got)
	}
}

func contains(s, sub string) bool {
	return stringIndex(s, sub) >= 0
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
