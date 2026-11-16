package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Response bodies below follow the shape of real Cloudflare API v4 responses
// (the update response is taken from the Kotlin version's log), with ids,
// names and addresses replaced by placeholders.
const (
	zonesResponse = `{"result":[` +
		`{"id":"zone-1","name":"example.com","status":"active"},` +
		`{"id":"zone-2","name":"a-much-longer-zone.example.org","status":"active"}],` +
		`"result_info":{"page":1,"per_page":20,"count":2,"total_count":2},` +
		`"success":true,"errors":[],"messages":[]}`

	dnsRecordsResponse = `{"result":[` +
		`{"id":"rec-music","name":"music.example.com","type":"A","content":"198.51.100.7","proxied":false,"ttl":120},` +
		`{"id":"rec-apex","name":"example.com","type":"A","content":"198.51.100.7","proxied":false,"ttl":120},` +
		`{"id":"rec-videos","name":"videos.example.com","type":"A","content":"198.51.100.7","proxied":true,"ttl":1}],` +
		`"result_info":{"page":1,"per_page":100,"count":3,"total_count":3},` +
		`"success":true,"errors":[],"messages":[]}`

	updateResponse = `{"result":{"id":"rec-music","name":"music.example.com","type":"A",` +
		`"content":"203.0.113.9","proxiable":true,"proxied":false,"ttl":120,"settings":{},"meta":{},` +
		`"comment":null,"tags":[],"created_on":"2025-05-25T13:46:56.660382Z",` +
		`"modified_on":"2026-11-08T03:55:22.540701Z"},"success":true,"errors":[],"messages":[]}`

	authErrorResponse = `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`
)

type recordedRequest struct {
	method string
	path   string
	header http.Header
	body   string
}

// fakeAPI is a local stand-in for the Cloudflare API.
type fakeAPI struct {
	server    *httptest.Server
	responses map[string]string // "METHOD /path" -> response body
	requests  []recordedRequest
}

func newFakeAPI(t *testing.T, responses map[string]string) *fakeAPI {
	t.Helper()
	api := &fakeAPI{responses: responses}
	api.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		api.requests = append(api.requests, recordedRequest{r.Method, r.URL.Path, r.Header, string(body)})

		response, ok := api.responses[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		io.WriteString(w, response)
	}))
	t.Cleanup(api.server.Close)
	return api
}

func (api *fakeAPI) calls() []string {
	var calls []string
	for _, r := range api.requests {
		calls = append(calls, r.method+" "+r.path)
	}
	return calls
}

func newTestCloudflare(t *testing.T, api *fakeAPI) (*Cloudflare, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	logger := NewLogger(out, filepath.Join(t.TempDir(), ".dns.log"))
	t.Cleanup(logger.close)
	return NewCloudflare(api.server.URL, "user@example.com", "secret-key", logger, out), out
}

func TestListZonesPrintsZoneNamesBetweenLines(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})
	cloudflare, out := newTestCloudflare(t, api)

	ok, err := cloudflare.listZones()

	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	want := "" +
		"──────────────────────────────\n" +
		"                   example.com\n" +
		"──────────────────────────────\n" +
		"a-much-longer-zone.example.org\n" +
		"──────────────────────────────\n" +
		"\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRequestsCarryAuthHeaders(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})
	cloudflare, _ := newTestCloudflare(t, api)

	cloudflare.listZones()

	header := api.requests[0].header
	if got := header.Get("X-Auth-Email"); got != "user@example.com" {
		t.Errorf("X-Auth-Email: got %q", got)
	}
	if got := header.Get("X-Auth-Key"); got != "secret-key" {
		t.Errorf("X-Auth-Key: got %q", got)
	}
	if got := header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q", got)
	}
}

func TestListZonesPrintsNothingWhenResponseHasNoResult(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": authErrorResponse})
	cloudflare, out := newTestCloudflare(t, api)

	ok, err := cloudflare.listZones()

	if err != nil || ok {
		t.Errorf("got ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	if out.String() != "" {
		t.Errorf("got output %q, want none", out.String())
	}
}

func TestListZonesPrintsBlankLineWhenThereAreNoZones(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": `{"result":[],"success":true}`})
	cloudflare, out := newTestCloudflare(t, api)

	ok, err := cloudflare.listZones()

	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	if out.String() != "\n" {
		t.Errorf("got %q, want a single blank line", out.String())
	}
}

func TestListZonesFailsWhenResponseIsNotJSON(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": "<html>Bad Gateway</html>"})
	cloudflare, _ := newTestCloudflare(t, api)

	_, err := cloudflare.listZones()

	if err == nil {
		t.Error("got no error, want one")
	}
}

func TestListZonesFailsWhenServerIsUnreachable(t *testing.T) {
	api := newFakeAPI(t, nil)
	cloudflare, _ := newTestCloudflare(t, api)
	api.server.Close()

	_, err := cloudflare.listZones()

	if err == nil {
		t.Error("got no error, want one")
	}
}

func TestListDNSRecordsByNamePrintsRecordNamesRightAligned(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                    zonesResponse,
		"GET /zones/zone-1/dns_records": dnsRecordsResponse,
	})
	cloudflare, out := newTestCloudflare(t, api)

	ok, err := cloudflare.listDNSRecordsByName("example.com", false)

	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	want := "" +
		" music.example.com\n" +
		"       example.com\n" +
		"videos.example.com\n" +
		"\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestListDNSRecordsByNameDrawsLinesWhenPretty(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                    zonesResponse,
		"GET /zones/zone-1/dns_records": dnsRecordsResponse,
	})
	cloudflare, out := newTestCloudflare(t, api)

	cloudflare.listDNSRecordsByName("example.com", true)

	want := "" +
		"──────────────────\n" +
		" music.example.com\n" +
		"──────────────────\n" +
		"       example.com\n" +
		"──────────────────\n" +
		"videos.example.com\n" +
		"──────────────────\n" +
		"\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestListDNSRecordsByNamePrintsNothingWhenZoneIsUnknown(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})
	cloudflare, out := newTestCloudflare(t, api)

	ok, err := cloudflare.listDNSRecordsByName("unknown.example", false)

	if err != nil || ok {
		t.Errorf("got ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	if out.String() != "" {
		t.Errorf("got output %q, want none", out.String())
	}
}

func TestUpdateRecordPutsWhenRecordExists(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                              zonesResponse,
		"GET /zones/zone-1/dns_records":           dnsRecordsResponse,
		"PUT /zones/zone-1/dns_records/rec-music": updateResponse,
	})
	cloudflare, _ := newTestCloudflare(t, api)

	ok, err := cloudflare.updateRecord("example.com", Param{"A", "music.example.com", "203.0.113.9", 120, false})

	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	wantCalls := []string{
		"GET /zones",
		"GET /zones/zone-1/dns_records",
		"PUT /zones/zone-1/dns_records/rec-music",
	}
	if !reflect.DeepEqual(api.calls(), wantCalls) {
		t.Errorf("calls: got %v, want %v", api.calls(), wantCalls)
	}
}

func TestUpdateRecordSendsRecordAsJSON(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                              zonesResponse,
		"GET /zones/zone-1/dns_records":           dnsRecordsResponse,
		"PUT /zones/zone-1/dns_records/rec-music": updateResponse,
	})
	cloudflare, _ := newTestCloudflare(t, api)

	cloudflare.updateRecord("example.com", Param{"A", "music.example.com", "203.0.113.9", 120, true})

	var got map[string]any
	if err := json.Unmarshal([]byte(api.requests[2].body), &got); err != nil {
		t.Fatalf("request body is not JSON: %q", api.requests[2].body)
	}
	want := map[string]any{
		"type":    "A",
		"name":    "music.example.com",
		"content": "203.0.113.9",
		"ttl":     float64(120),
		"proxied": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUpdateRecordPostsWhenRecordDoesNotExist(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                     zonesResponse,
		"GET /zones/zone-1/dns_records":  dnsRecordsResponse,
		"POST /zones/zone-1/dns_records": updateResponse,
	})
	cloudflare, _ := newTestCloudflare(t, api)

	ok, err := cloudflare.updateRecord("example.com", Param{"A", "new.example.com", "203.0.113.9", 120, false})

	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	if got := api.calls()[2]; got != "POST /zones/zone-1/dns_records" {
		t.Errorf("got %q", got)
	}
}

func TestUpdateRecordLogsRawResponseThenPrintsItIndented(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                              zonesResponse,
		"GET /zones/zone-1/dns_records":           dnsRecordsResponse,
		"PUT /zones/zone-1/dns_records/rec-music": updateResponse,
	})
	cloudflare, out := newTestCloudflare(t, api)

	cloudflare.updateRecord("example.com", Param{"A", "music.example.com", "203.0.113.9", 120, false})

	logLine, printed, _ := strings.Cut(out.String(), "\n")
	if !strings.HasSuffix(logLine, " INFO  music.example.com - "+updateResponse) {
		t.Errorf("log line: got %q", logLine)
	}
	wantPrinted := `{
  "result": {
    "id": "rec-music",
    "name": "music.example.com",
    "type": "A",
    "content": "203.0.113.9",
    "proxiable": true,
    "proxied": false,
    "ttl": 120,
    "settings": {},
    "meta": {},
    "comment": null,
    "tags": [],
    "created_on": "2025-05-25T13:46:56.660382Z",
    "modified_on": "2026-11-08T03:55:22.540701Z"
  },
  "success": true,
  "errors": [],
  "messages": []
}
`
	if printed != wantPrinted {
		t.Errorf("printed:\n%s\nwant:\n%s", printed, wantPrinted)
	}
}

func TestUpdateRecordWarnsAndSendsNothingWhenZoneIsUnknown(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": authErrorResponse})
	cloudflare, out := newTestCloudflare(t, api)

	ok, err := cloudflare.updateRecord("example.com", Param{"A", "music.example.com", "203.0.113.9", 120, false})

	if err != nil || ok {
		t.Errorf("got ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	if !reflect.DeepEqual(api.calls(), []string{"GET /zones"}) {
		t.Errorf("calls: got %v", api.calls())
	}
	if !strings.HasSuffix(out.String(), " WARN  music.example.com - Update failed: zone example.com not found\n") {
		t.Errorf("got %q", out.String())
	}
}

func TestUpdateRecordWarnsWhenResponseIsEmpty(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                              zonesResponse,
		"GET /zones/zone-1/dns_records":           dnsRecordsResponse,
		"PUT /zones/zone-1/dns_records/rec-music": "",
	})
	cloudflare, out := newTestCloudflare(t, api)

	ok, err := cloudflare.updateRecord("example.com", Param{"A", "music.example.com", "203.0.113.9", 120, false})

	if err != nil || ok {
		t.Errorf("got ok=%v err=%v, want ok=false err=nil", ok, err)
	}
	if !strings.HasSuffix(out.String(), " WARN  music.example.com - Update failed: empty response\n") {
		t.Errorf("got %q", out.String())
	}
}

func TestUpdateRecordFailsWhenResponseIsNotJSON(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                              zonesResponse,
		"GET /zones/zone-1/dns_records":           dnsRecordsResponse,
		"PUT /zones/zone-1/dns_records/rec-music": "<html>Bad Gateway</html>",
	})
	cloudflare, _ := newTestCloudflare(t, api)

	_, err := cloudflare.updateRecord("example.com", Param{"A", "music.example.com", "203.0.113.9", 120, false})

	if err == nil {
		t.Error("got no error, want one")
	}
}

func TestGetIPReturnsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "203.0.113.9")
	}))
	defer server.Close()

	ip, err := getIP(server.URL)

	if err != nil || ip != "203.0.113.9" {
		t.Errorf("got ip=%q err=%v", ip, err)
	}
}

func TestGetIPFailsWhenServerIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	_, err := getIP(server.URL)

	if err == nil {
		t.Error("got no error, want one")
	}
}
