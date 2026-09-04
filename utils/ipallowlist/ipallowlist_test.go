package ipallowlist

import "testing"

func TestContains(t *testing.T) {
	list := "192.168.0.0/16,10.0.0.0/8"
	if !Contains("192.168.1.1", list, false) {
		t.Fatal("expected match")
	}
	if Contains("172.16.0.1", list, false) {
		t.Fatal("expected no match")
	}
	if Contains("192.168.1.1", "", false) {
		t.Fatal("empty list must not match")
	}
	if !Contains("@", "@", true) {
		t.Fatal("@ entry must match unix peer")
	}
}
