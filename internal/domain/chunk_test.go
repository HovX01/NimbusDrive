package domain

import "testing"

func TestSplitChunks(t *testing.T) {
	t.Parallel()
	got := SplitChunks([]byte("abcdefghij"), 3)
	if len(got) != 4 {
		t.Fatalf("len=%d", len(got))
	}
	if string(got[0]) != "abc" || string(got[3]) != "j" {
		t.Fatalf("unexpected chunks: %#v", got)
	}
	empty := SplitChunks(nil, 8)
	if len(empty) != 1 || len(empty[0]) != 0 {
		t.Fatalf("empty: %#v", empty)
	}
}

func TestMapRangeToParts(t *testing.T) {
	t.Parallel()
	sizes := []int64{10, 10, 5}
	ranges, err := MapRangeToParts(sizes, 8, 22)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 3 {
		t.Fatalf("len=%d %#v", len(ranges), ranges)
	}
	if ranges[0].PartNo != 0 || ranges[0].Skip != 8 || ranges[0].Take != 2 {
		t.Fatalf("part0 %#v", ranges[0])
	}
	if ranges[1].Skip != 0 || ranges[1].Take != 10 {
		t.Fatalf("part1 %#v", ranges[1])
	}
	if ranges[2].Take != 3 {
		t.Fatalf("part2 %#v", ranges[2])
	}
}

func TestValidateNodeName(t *testing.T) {
	t.Parallel()
	if err := ValidateNodeName("ok.txt"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNodeName("../x"); err == nil {
		t.Fatal("expected error")
	}
}
