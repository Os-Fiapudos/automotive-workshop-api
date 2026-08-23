package trackingtoken

import "testing"

func TestGenerateLength(t *testing.T) {
	token, err := Generate()
	if err != nil {
		t.Fatalf("Generate returned an error: %v", err)
	}
	if len(token) != rawByteLength*2 {
		t.Fatalf("expected a %d-character hex string, got %d characters: %q", rawByteLength*2, len(token), token)
	}
}

func TestGenerateIsRandom(t *testing.T) {
	first, err := Generate()
	if err != nil {
		t.Fatalf("Generate returned an error: %v", err)
	}
	second, err := Generate()
	if err != nil {
		t.Fatalf("Generate returned an error: %v", err)
	}
	if first == second {
		t.Fatalf("two consecutive calls to Generate returned the same token: %q", first)
	}
}

func TestHashIsDeterministic(t *testing.T) {
	raw := "sample-token-value"
	if Hash(raw) != Hash(raw) {
		t.Fatalf("Hash(%q) was not deterministic across calls", raw)
	}
}

func TestHashDiffersForDifferentInputs(t *testing.T) {
	if Hash("token-a") == Hash("token-b") {
		t.Fatal("Hash produced the same digest for two different inputs")
	}
}

func TestHashLength(t *testing.T) {
	hash := Hash("sample-token-value")
	const sha256HexLength = 64
	if len(hash) != sha256HexLength {
		t.Fatalf("expected a %d-character hex string, got %d characters: %q", sha256HexLength, len(hash), hash)
	}
}
