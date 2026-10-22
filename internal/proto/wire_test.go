package proto

import (
	"bytes"
	"testing"
)

func TestWireRoundTrip(t *testing.T) {
	b := EncodeWire(0x0100, []byte{1, 0, 0, 0})
	if len(b) != 12 || b[0] != 12 || b[2] != 12 || b[4] != 0x00 || b[5] != 0x01 {
		t.Fatalf("bad header % x", b)
	}
	f, err := DecodeWire(bytes.NewReader(b))
	if err != nil || f.Cmd != 0x0100 || !bytes.Equal(f.Payload, []byte{1, 0, 0, 0}) {
		t.Fatalf("got %+v %v", f, err)
	}
}

func TestChecksum(t *testing.T) {
	// i=0:1+0, i=1:-1+1, i=2:2+2 => 5
	if c := Checksum([]byte{1, 0xff, 2}); c != 5 {
		t.Fatalf("got %d", c)
	}
}
