package zonefile

import (
	"fmt"
	"strings"

	"zonemeister/internal/netnod"

	"github.com/miekg/dns"
)

// Parse reads BIND zone file content and returns RRsets suitable for the
// Netnod API. SOA records are skipped since the API manages those. The origin
// parameter should be the zone name (e.g. "example.com.").
func Parse(origin string, content string) ([]netnod.RRset, error) {
	if !strings.HasSuffix(origin, ".") {
		origin += "."
	}

	parser := dns.NewZoneParser(strings.NewReader(content), origin, "")

	type rrsetKey struct {
		Name string
		Type string
	}

	grouped := make(map[rrsetKey]*netnod.RRset)
	var order []rrsetKey

	for rr, ok := parser.Next(); ok; rr, ok = parser.Next() {
		header := rr.Header()

		// Skip SOA records — managed by the API.
		if header.Rrtype == dns.TypeSOA {
			continue
		}

		typeName := dns.TypeToString[header.Rrtype]
		if typeName == "" {
			continue
		}

		// Extract the rdata portion (everything after the header).
		full := rr.String()
		// The header is "name TTL CLASS TYPE", rdata follows after.
		parts := strings.Fields(full)
		if len(parts) < 5 {
			continue
		}
		// Rejoin from field index 4 onward (the rdata).
		rdata := strings.Join(parts[4:], " ")

		ttl := int(header.Ttl)
		key := rrsetKey{Name: header.Name, Type: typeName}

		if existing, ok := grouped[key]; ok {
			existing.Records = append(existing.Records, netnod.Record{Content: rdata})
		} else {
			rrset := &netnod.RRset{
				Name: header.Name,
				Type: typeName,
				TTL:  &ttl,
				Records: []netnod.Record{
					{Content: rdata},
				},
			}
			grouped[key] = rrset
			order = append(order, key)
		}
	}

	if err := parser.Err(); err != nil {
		return nil, fmt.Errorf("parse zone file: %w", err)
	}

	result := make([]netnod.RRset, 0, len(order))
	for _, key := range order {
		result = append(result, *grouped[key])
	}

	return result, nil
}
