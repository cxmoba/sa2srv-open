package mbcs

import (
	"bytes"
	"testing"
)

func TestGBKRoundTrip(t *testing.T) {
	b := EncodeAs(GBK, "中文a")
	if !bytes.Equal(b, []byte{0xd6, 0xd0, 0xce, 0xc4, 'a'}) {
		t.Fatalf("encode % x", b)
	}
	if s := DecodeAs(GBK, b); s != "中文a" {
		t.Fatalf("decode %q", s)
	}
	if s := DecodeAs(Raw, []byte("abc")); s != "abc" {
		t.Fatal(s)
	}
}

func TestFit(t *testing.T) {
	if b := Fit(GBK, "中文字", 5); len(b) != 4 {
		t.Fatalf("fit len %d", len(b))
	}
	if b := Fit(Raw, "abcdef", 3); string(b) != "abc" {
		t.Fatal(string(b))
	}
}
