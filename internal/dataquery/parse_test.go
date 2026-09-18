package dataquery

import (
	"net/http"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	req := mustReq("?price=gte.10&name=like.*shirt*&order=price.desc&limit=5&offset=2")
	q, err := Parse(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Filters) != 2 || q.Limit != 5 || q.Offset != 2 || q.OrderBy != "price" || !q.Desc {
		t.Fatalf("unexpected query: %+v", q)
	}
}

func TestValidCollection(t *testing.T) {
	t.Parallel()
	if !ValidCollection("products") || ValidCollection("9bad") || ValidCollection("") {
		t.Fatal("collection validation failed")
	}
}

func mustReq(raw string) *http.Request {
	r, _ := http.NewRequest("GET", "http://x/"+raw, nil)
	return r
}
