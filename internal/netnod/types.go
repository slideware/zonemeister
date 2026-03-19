package netnod

// ZoneListResponse is the paginated response from GET /api/v1/zones.
type ZoneListResponse struct {
	Data   []ZoneSummary `json:"data"`
	Offset int           `json:"offset"`
	Limit  int           `json:"limit"`
	Total  int           `json:"total"`
}

// ZoneSummary is a zone entry in the list response.
type ZoneSummary struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	NotifiedSerial int    `json:"notified_serial"`
}

// Zone is the full zone detail returned by GET /api/v1/zones/{zoneId}.
type Zone struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	NotifiedSerial    int      `json:"notified_serial"`
	AlsoNotify        []string `json:"also_notify"`
	AllowTransferKeys []string `json:"allow_transfer_keys"`
	RRsets            []RRset  `json:"rrsets"`
}

// RRset represents a DNS resource record set.
type RRset struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     *int     `json:"ttl,omitempty"`
	Records []Record `json:"records"`
}

// Record is a single DNS record within an RRset.
type Record struct {
	Content  string `json:"content"`
	Disabled bool   `json:"disabled"`
}

// RRsetChange describes a change to an RRset used in PATCH operations.
type RRsetChange struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	ChangeType string   `json:"changetype"`
	TTL        *int     `json:"ttl,omitempty"`
	Records    []Record `json:"records,omitempty"`
}

// CreateZoneRequest is the request body for POST /api/v1/zones.
type CreateZoneRequest struct {
	Name              string   `json:"name"`
	RRsets            []RRset  `json:"rrsets,omitempty"`
	Zone              string   `json:"zone,omitempty"`
	AlsoNotify        []string `json:"also_notify,omitempty"`
	AllowTransferKeys []string `json:"allow_transfer_keys,omitempty"`
}

// UpdateZoneRequest is the request body for PUT /api/v1/zones/{zoneId}.
type UpdateZoneRequest struct {
	RRsets            []RRset  `json:"rrsets,omitempty"`
	AlsoNotify        []string `json:"also_notify,omitempty"`
	AllowTransferKeys []string `json:"allow_transfer_keys,omitempty"`
}

// PatchZoneRequest is the request body for PATCH /api/v1/zones/{zoneId}.
type PatchZoneRequest struct {
	RRsets []RRsetChange `json:"rrsets"`
}

// DynDNSLabel is a DynDNS label entry.
type DynDNSLabel struct {
	Label    string `json:"label"`
	Hostname string `json:"hostname"`
}

// DynDNSListResponse is the response from GET /api/v1/zones/{zoneId}/dyndns.
type DynDNSListResponse struct {
	Labels []DynDNSLabel `json:"labels"`
}

// DynDNSEnableResponse is the response from POST /api/v1/zones/{zoneId}/dyndns/{label}.
type DynDNSEnableResponse struct {
	Hostname string `json:"hostname"`
	Token    string `json:"token"`
}

// ACMELabel is an ACME label entry.
type ACMELabel struct {
	Label             string `json:"label"`
	Hostname          string `json:"hostname"`
	ChallengeHostname string `json:"challenge_hostname"`
}

// ACMEListResponse is the response from GET /api/v1/zones/{zoneId}/acme.
type ACMEListResponse struct {
	Labels []ACMELabel `json:"labels"`
}

// ACMEEnableResponse is the response from POST /api/v1/zones/{zoneId}/acme/{label}.
type ACMEEnableResponse struct {
	Hostname          string `json:"hostname"`
	ChallengeHostname string `json:"challenge_hostname"`
	Token             string `json:"token"`
}

// APIError represents an error response from the Netnod API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return e.Message
}
