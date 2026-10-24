package crypto

import (
	"bytes"
	"errors"
	"testing"
)

func TestEnvelope(t *testing.T) {
	plain := []byte(`{"private":"secret payload"}`)
	sealed, err := Seal(plain, "test-only passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if formatVersion, err := Inspect(sealed); err != nil || formatVersion != 1 {
		t.Fatalf("inspect envelope: version=%d err=%v", formatVersion, err)
	}
	got, err := Open(sealed, "test-only passphrase")
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("decrypt: %v", err)
	}
	other, err := Seal(plain, "test-only passphrase")
	if err != nil || bytes.Equal(sealed, other) {
		t.Fatal("salt/nonce were reused")
	}
	if _, err := Open(sealed, "wrong passphrase"); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	for _, index := range []int{8, 24, len(sealed) - 1} {
		corrupt := bytes.Clone(sealed)
		corrupt[index] ^= 1
		if _, err := Open(corrupt, "test-only passphrase"); !errors.Is(err, ErrAuthentication) {
			t.Fatalf("tamper at %d: %v", index, err)
		}
	}
}

func TestInspectReportsUnsupportedEnvelopeVersion(t *testing.T) {
	sealed, err := Seal([]byte("payload"), "test-only passphrase")
	if err != nil {
		t.Fatal(err)
	}
	sealed[4] = 2
	version, err := Inspect(sealed)
	if err != nil || version != 2 {
		t.Fatalf("inspect future format: version=%d err=%v", version, err)
	}
	if _, err := Open(sealed, "test-only passphrase"); !errors.Is(err, ErrFormat) {
		t.Fatalf("Open accepted unsupported project format: %v", err)
	}
}

func TestInvalidEnvelope(t *testing.T) {
	for _, input := range [][]byte{nil, []byte("not an envelope"), make([]byte, 60)} {
		if _, err := Open(input, "test"); !errors.Is(err, ErrFormat) {
			t.Fatal("accepted invalid format")
		}
	}
	if _, err := Seal([]byte("data"), ""); err == nil {
		t.Fatal("accepted empty passphrase")
	}
}
