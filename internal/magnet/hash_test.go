package magnet

import "testing"

func TestParseHashAcceptsHex(t *testing.T) {
	hash, err := ParseHash("30291C52BB72D46AFFC1574EC01A4E16FC28A292")
	if err != nil {
		t.Fatalf("ParseHash() error = %v", err)
	}
	if hash != "30291c52bb72d46affc1574ec01a4e16fc28a292" {
		t.Fatalf("ParseHash() = %q", hash)
	}
}

func TestParseHashExtractsFromMagnetURI(t *testing.T) {
	uri := "magnet:?xt=urn:btih:FCEf4eb87ac04a85c5285f8377b2856e9e40b9f1&dn=SSIS-001"
	hash, err := ParseHash(uri)
	if err != nil {
		t.Fatalf("ParseHash() error = %v", err)
	}
	if hash != "fcef4eb87ac04a85c5285f8377b2856e9e40b9f1" {
		t.Fatalf("ParseHash() = %q", hash)
	}
}

func TestParseHashDecodesBase32(t *testing.T) {
	// base32("fcef4eb87ac04a85c5285f8377b2856e9e40b9f1") without padding.
	hash, err := ParseHash("magnet:?xt=urn:btih:7txu5od2ybfilrjil6bxpmufn2pebopr")
	if err != nil {
		t.Fatalf("ParseHash() error = %v", err)
	}
	if hash != "fcef4eb87ac04a85c5285f8377b2856e9e40b9f1" {
		t.Fatalf("ParseHash() = %q", hash)
	}
}

func TestParseHashRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "   ", "not-a-hash", "magnet:?dn=only", "12345"} {
		if _, err := ParseHash(input); err == nil {
			t.Fatalf("ParseHash(%q) expected error", input)
		}
	}
}
