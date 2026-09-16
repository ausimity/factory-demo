package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// legacySeededProjection is the exact ordered pre-feature public projection for
// the seeded cust-1001 portfolio (customerId, asOf, totalMarketValue, accounts).
// Removing only the additive insights member from a post-feature response must
// reproduce these bytes exactly.
const legacySeededProjection = `{"customerId":"cust-1001","asOf":"2026-09-05T12:00:00Z","totalMarketValue":{"amount":"34375.00","currency":"USD"},"accounts":[{"accountId":"acct-brokerage-01","accountType":"BROKERAGE","cashBalance":{"amount":"2500.00","currency":"USD"},"marketValue":{"amount":"21025.00","currency":"USD"},"holdings":[{"symbol":"AAPL","quantity":"100","unitPrice":{"amount":"185.25","currency":"USD"},"marketValue":{"amount":"18525.00","currency":"USD"}}]},{"accountId":"acct-retirement-01","accountType":"RETIREMENT","cashBalance":{"amount":"2600.00","currency":"USD"},"marketValue":{"amount":"13350.00","currency":"USD"},"holdings":[{"symbol":"BND","quantity":"200","unitPrice":{"amount":"53.75","currency":"USD"},"marketValue":{"amount":"10750.00","currency":"USD"}}]}]}`

type rawMember struct {
	key string
	raw json.RawMessage
}

// topLevelMembers decodes a JSON object into its ordered members, preserving the
// exact raw bytes of each value so a projection can be reconstructed without
// reserialization artifacts.
func topLevelMembers(t *testing.T, raw []byte) []rawMember {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("read opening token: %v", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		t.Fatalf("expected JSON object, got %v", tok)
	}
	var members []rawMember
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			t.Fatalf("read key token: %v", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			t.Fatalf("object key is not a string: %v", keyTok)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			t.Fatalf("decode value for %q: %v", key, err)
		}
		members = append(members, rawMember{key: key, raw: value})
	}
	return members
}

func projectionWithout(members []rawMember, omit string) string {
	parts := make([]string, 0, len(members))
	for _, member := range members {
		if member.key == omit {
			continue
		}
		parts = append(parts, fmt.Sprintf("%q:%s", member.key, member.raw))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// TestFullPathRemovingInsightsYieldsLegacyProjection proves the post-feature
// response places insights last as an additive member and that removing only
// insights reproduces the exact ordered legacy projection byte-for-byte.
func TestFullPathRemovingInsightsYieldsLegacyProjection(t *testing.T) {
	t.Parallel()

	path := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV2Body))
	recorder := getPortfolio(path.handler, "cust-1001")
	assertStatus(t, recorder, http.StatusOK)

	members := topLevelMembers(t, recorder.Body.Bytes())
	gotKeys := make([]string, 0, len(members))
	for _, member := range members {
		gotKeys = append(gotKeys, member.key)
	}
	wantKeys := []string{"customerId", "asOf", "totalMarketValue", "accounts", "insights"}
	if strings.Join(gotKeys, ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("top-level keys = %v, want %v", gotKeys, wantKeys)
	}

	if got := projectionWithout(members, "insights"); got != legacySeededProjection {
		t.Fatalf("legacy projection mismatch:\n got=%s\nwant=%s", got, legacySeededProjection)
	}
}

// TestFullPathV1V2ByteEqualityWithInsights proves the complete post-feature
// responses (including insights) for equivalent Core v1 and v2 inputs are
// byte-identical, each issuing exactly one Core and one Market call and carrying
// a populated insights array.
func TestFullPathV1V2ByteEqualityWithInsights(t *testing.T) {
	t.Parallel()

	v1 := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV1Body))
	v2 := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV2Body))

	v1Response := getPortfolio(v1.handler, "cust-1001")
	v2Response := getPortfolio(v2.handler, "cust-1001")

	assertStatus(t, v1Response, http.StatusOK)
	assertStatus(t, v2Response, http.StatusOK)
	assertCalls(t, v1, 1, 1)
	assertCalls(t, v2, 1, 1)

	if !bytes.Equal(v1Response.Body.Bytes(), v2Response.Body.Bytes()) {
		t.Fatalf("v1 and v2 complete output differ:\nv1=%s\nv2=%s", v1Response.Body.Bytes(), v2Response.Body.Bytes())
	}

	var decoded struct {
		Insights []map[string]any `json:"insights"`
	}
	if err := json.Unmarshal(v2Response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode insights: %v", err)
	}
	if len(decoded.Insights) == 0 {
		t.Fatalf("expected a populated insights array, got %s", v2Response.Body.Bytes())
	}
}

// TestFullPathErrorResponsesOmitInsights proves invalid-ID 400, not-found 404,
// and upstream 502 responses retain the exact error envelope and never contain
// an insights member.
func TestFullPathErrorResponsesOmitInsights(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		customerID string
		coreFn     http.HandlerFunc
		wantStatus int
	}{
		{name: "invalid id 400", customerID: "not-a-customer", coreFn: staticCore(http.StatusOK, fullPathV2Body), wantStatus: http.StatusBadRequest},
		{name: "not found 404", customerID: "cust-9999", coreFn: staticCore(http.StatusNotFound, `{}`), wantStatus: http.StatusNotFound},
		{name: "upstream 502", customerID: "cust-1001", coreFn: staticCore(http.StatusInternalServerError, `{"secret":"x"}`), wantStatus: http.StatusBadGateway},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := newFullPath(t, nil, tc.coreFn)
			recorder := getPortfolio(path.handler, tc.customerID)
			assertStatus(t, recorder, tc.wantStatus)
			assertSecurityHeaders(t, recorder)
			if strings.Contains(recorder.Body.String(), "insights") {
				t.Fatalf("error response leaked insights member: %s", recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"error"`) {
				t.Fatalf("error response missing error envelope: %s", recorder.Body.String())
			}
		})
	}
}
