package encoding

import (
	"bytes"
	"testing"
)

func TestAllBytes(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	got, err := Decode(Encode(data))
	if err != nil || !bytes.Equal(data, got) {
		t.Fatalf("roundtrip failed: %v", err)
	}
	if Encode([]byte{0x01, 0xef}) != "rizz skibidi goated bet" {
		t.Fatal("V1 vocabulary changed")
	}
}

func TestInvalidEncoding(t *testing.T) {
	for _, text := range []string{"rizz", "rizz nope", "nope rizz"} {
		if _, err := Decode(text); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}

func FuzzRoundTrip(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("no cap"))
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := Decode(Encode(data))
		if err != nil || !bytes.Equal(data, got) {
			t.Fatalf("roundtrip failed: %v", err)
		}
	})
}

func FuzzDecode(f *testing.F) {
	f.Add("")
	f.Add("rizz skibidi")
	f.Add("rizz nope")
	f.Add("rizz skibidi gyatt")
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 1<<20 {
			t.Skip()
		}
		_, _ = Decode(text)
	})
}
