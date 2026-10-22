package proto

import (
	"bytes"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	orig := Frame{
		Seq:     0x1234,
		Len:     HeaderSize + 6,
		Cmd:     0x0410,
		Payload: []byte{0xAA, 0xCC, 0xDE, 0xAD, 0xBE, 0xEF},
	}
	buf := Encode(orig)
	if len(buf) != int(orig.Len) {
		t.Fatalf("len got %d want %d", len(buf), orig.Len)
	}

	got, err := Decode(bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != orig.Seq || got.Len != orig.Len || got.Cmd != orig.Cmd || !bytes.Equal(got.Payload, orig.Payload) {
		t.Fatalf("round-trip got %+v want %+v", got, orig)
	}
}

func TestReadFullShortHdr(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte{1, 2}))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestReadFullBadLen(t *testing.T) {
	// len=3 < HeaderSize
	buf := []byte{1, 2, 3, 4, 5, 0} // len 字段 = 3
	_, err := Decode(bytes.NewReader(buf))
	if err == nil {
		t.Fatal("want short hdr error")
	}
}

func TestReadFullTrailing(t *testing.T) {
	// 验证 Decode 只读一个包, 剩余留在 buf
	orig := Frame{Seq: 1, Cmd: 0x0410, Payload: []byte("hi")}
	buf := Encode(orig)
	buf = append(buf, []byte{0xff, 0xff}...) // 后续噪声

	got, err := Decode(bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Payload) != "hi" {
		t.Fatalf("payload got %q want hi", got.Payload)
	}
}
