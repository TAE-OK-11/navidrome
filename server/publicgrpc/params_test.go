package publicgrpc

import "testing"

func TestMapToURLValuesRepeated(t *testing.T) {
	q := mapToURLValues(map[string]string{
		"id": "1" + paramValueSep + "2",
		"f":  "json",
	})
	if got := q["id"]; len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("id=%v", got)
	}
	if q.Get("f") != "json" {
		t.Fatalf("f=%q", q.Get("f"))
	}
}
