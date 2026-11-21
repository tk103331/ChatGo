package main

import "testing"

func TestParsePairDeepLink(t *testing.T) {
	u, err := parseDeepLink("chatgo://pair?v=1&r=wss%3A%2F%2Frelay.example.com&d=d1&c=ABCDEFGH")
	if err != nil {
		t.Fatal(err)
	}
	if u.kind != "pair" || u.relay != "wss://relay.example.com" || u.desktop != "d1" || u.code != "ABCDEFGH" {
		t.Fatalf("link = %+v", u)
	}
}

func TestParseSessionDeepLink(t *testing.T) {
	u, err := parseDeepLink("chatgo://session/c1")
	if err != nil {
		t.Fatal(err)
	}
	if u.kind != "session" || u.session != "c1" {
		t.Fatalf("link = %+v", u)
	}
}
