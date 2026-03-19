package zonefile

import (
	"testing"
)

func TestParse_BasicRecords(t *testing.T) {
	content := `$ORIGIN example.com.
$TTL 3600
example.com. IN A 1.2.3.4
example.com. IN A 5.6.7.8
example.com. IN AAAA 2001:db8::1
www          IN A 9.10.11.12
`
	rrsets, err := Parse("example.com", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rrsets) != 3 {
		t.Fatalf("expected 3 rrsets, got %d", len(rrsets))
	}

	// First rrset: example.com. A with 2 records
	if rrsets[0].Name != "example.com." || rrsets[0].Type != "A" {
		t.Errorf("rrset[0]: expected example.com. A, got %s %s", rrsets[0].Name, rrsets[0].Type)
	}
	if len(rrsets[0].Records) != 2 {
		t.Errorf("rrset[0]: expected 2 records, got %d", len(rrsets[0].Records))
	}
	if rrsets[0].Records[0].Content != "1.2.3.4" {
		t.Errorf("rrset[0].records[0]: expected 1.2.3.4, got %s", rrsets[0].Records[0].Content)
	}

	// Second rrset: example.com. AAAA
	if rrsets[1].Name != "example.com." || rrsets[1].Type != "AAAA" {
		t.Errorf("rrset[1]: expected example.com. AAAA, got %s %s", rrsets[1].Name, rrsets[1].Type)
	}

	// Third rrset: www.example.com. A
	if rrsets[2].Name != "www.example.com." || rrsets[2].Type != "A" {
		t.Errorf("rrset[2]: expected www.example.com. A, got %s %s", rrsets[2].Name, rrsets[2].Type)
	}
}

func TestParse_SkipsSOA(t *testing.T) {
	content := `$ORIGIN example.com.
example.com. 3600 IN SOA ns1.example.com. hostmaster.example.com. 2025010101 3600 900 604800 300
example.com. 3600 IN NS ns1.example.com.
`
	rrsets, err := Parse("example.com.", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rrsets) != 1 {
		t.Fatalf("expected 1 rrset (SOA skipped), got %d", len(rrsets))
	}
	if rrsets[0].Type != "NS" {
		t.Errorf("expected NS, got %s", rrsets[0].Type)
	}
}

func TestParse_MXRecord(t *testing.T) {
	content := `$ORIGIN example.com.
example.com. 3600 IN MX 10 mail.example.com.
example.com. 3600 IN MX 20 mail2.example.com.
`
	rrsets, err := Parse("example.com.", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rrsets) != 1 {
		t.Fatalf("expected 1 rrset, got %d", len(rrsets))
	}
	if len(rrsets[0].Records) != 2 {
		t.Fatalf("expected 2 MX records, got %d", len(rrsets[0].Records))
	}
	if rrsets[0].Records[0].Content != "10 mail.example.com." {
		t.Errorf("unexpected MX content: %s", rrsets[0].Records[0].Content)
	}
}

func TestParse_TXTRecord(t *testing.T) {
	content := `$ORIGIN example.com.
example.com. 3600 IN TXT "v=spf1 include:_spf.google.com ~all"
`
	rrsets, err := Parse("example.com.", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rrsets) != 1 {
		t.Fatalf("expected 1 rrset, got %d", len(rrsets))
	}
	if rrsets[0].Type != "TXT" {
		t.Errorf("expected TXT, got %s", rrsets[0].Type)
	}
}

func TestParse_TTLPreserved(t *testing.T) {
	content := `$ORIGIN example.com.
www.example.com. 7200 IN A 1.2.3.4
`
	rrsets, err := Parse("example.com.", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rrsets[0].TTL == nil || *rrsets[0].TTL != 7200 {
		t.Errorf("expected TTL 7200, got %v", rrsets[0].TTL)
	}
}

func TestParse_EmptyContent(t *testing.T) {
	rrsets, err := Parse("example.com.", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rrsets) != 0 {
		t.Errorf("expected 0 rrsets, got %d", len(rrsets))
	}
}

func TestParse_MalformedInput(t *testing.T) {
	content := `this is not a valid zone file !!!`
	_, err := Parse("example.com.", content)
	if err == nil {
		t.Error("expected error for malformed input")
	}
}

func TestParse_AddsTrailingDotToOrigin(t *testing.T) {
	content := `$TTL 3600
@ IN A 1.2.3.4
`
	rrsets, err := Parse("example.com", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rrsets) != 1 {
		t.Fatalf("expected 1 rrset, got %d", len(rrsets))
	}
	if rrsets[0].Name != "example.com." {
		t.Errorf("expected example.com., got %s", rrsets[0].Name)
	}
}
