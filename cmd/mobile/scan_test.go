package main

import "testing"

func TestPairQRCodeFormats(t *testing.T) {
	for _, raw := range []string{
		"chatgo://pair?v=1&r=ws%3A%2F%2F82.156.132.143%3A8080&d=d1&c=abcdefgh",
		"http://82.156.132.143:8080/pair#v=1&r=ws%3A%2F%2F82.156.132.143%3A8080&d=d1&c=ABCDEFGH",
	} {
		link, err := parsePairQR(raw)
		if err != nil {
			t.Fatal(err)
		}
		if link.relay != "ws://82.156.132.143:8080" || link.desktop != "d1" || link.code != "ABCDEFGH" {
			t.Fatalf("wrong pairing fields: %+v", link)
		}
	}
}

func TestPairQRCodeRejectsOtherContent(t *testing.T) {
	for _, raw := range []string{
		"hello", "https://example.com/", "chatgo://session/c1",
		"other://pair?r=wss://example.com&d=d1&c=ABCDEFGH",
		"chatgo://pair?r=file:///tmp&d=d1&c=ABCDEFGH",
		"chatgo://pair?r=wss://example.com&d=d1",
		"chatgo://pair?r=wss://example.com&d=../d1&c=ABCDEFGH",
		"chatgo://pair?r=wss://example.com&d=d1&c=ABCD!!!H",
	} {
		if _, err := parsePairQR(raw); err == nil {
			t.Fatalf("accepted invalid QR: %q", raw)
		}
	}
}
