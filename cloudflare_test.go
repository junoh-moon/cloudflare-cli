package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// Response bodies below follow the shape of real Cloudflare API v4 responses
// (the update response is taken from a real log), with ids, names and
// addresses replaced by placeholders.
const (
	zonesResponse = `{"result":[` +
		`{"id":"zone-1","name":"example.com","status":"active"},` +
		`{"id":"zone-2","name":"a-much-longer-zone.example.org","status":"active"}],` +
		`"result_info":{"page":1,"per_page":20,"count":2,"total_count":2,"total_pages":1},` +
		`"success":true,"errors":[],"messages":[]}`

	// The zone itself has an MX record next to its A record, listed first.
	// music and the zone point at an old address; videos is already current.
	dnsRecordsResponse = `{"result":[` +
		`{"id":"rec-mx","name":"example.com","type":"MX","content":"example.com","priority":0,"proxied":false,"ttl":1},` +
		`{"id":"rec-music","name":"music.example.com","type":"A","content":"198.51.100.7","proxied":false,"ttl":120},` +
		`{"id":"rec-apex","name":"example.com","type":"A","content":"198.51.100.7","proxied":false,"ttl":120},` +
		`{"id":"rec-videos","name":"videos.example.com","type":"A","content":"203.0.113.9","proxied":false,"ttl":120}],` +
		`"result_info":{"page":1,"per_page":100,"count":4,"total_count":4,"total_pages":1},` +
		`"success":true,"errors":[],"messages":[]}`

	updateResponse = `{"result":{"id":"rec-music","name":"music.example.com","type":"A",` +
		`"content":"203.0.113.9","proxiable":true,"proxied":false,"ttl":120,"settings":{},"meta":{},` +
		`"comment":null,"tags":[],"created_on":"2025-05-25T13:46:56.660382Z",` +
		`"modified_on":"2026-11-08T03:55:22.540701Z"},"success":true,"errors":[],"messages":[]}`

	refusedResponse = `{"result":null,"success":false,` +
		`"errors":[{"code":9005,"message":"Content for A record must be a valid IPv4 address."}],"messages":[]}`

	authErrorResponse = `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`
)

// existingRecords is dnsRecordsResponse as the client parses it.
var existingRecords = []Record{
	{ID: "rec-mx", Name: "example.com", Type: "MX", Content: "example.com", TTL: 1},
	{ID: "rec-music", Name: "music.example.com", Type: "A", Content: "198.51.100.7", TTL: 120},
	{ID: "rec-apex", Name: "example.com", Type: "A", Content: "198.51.100.7", TTL: 120},
	{ID: "rec-videos", Name: "videos.example.com", Type: "A", Content: "203.0.113.9", TTL: 120},
}

type recordedRequest struct {
	method string
	path   string
	query  string
	header http.Header
	body   string
}

// fakeAPI is a local stand-in for the Cloudflare API.
type fakeAPI struct {
	server *httptest.Server
	// "METHOD /path" -> response body. A key may also carry a query string
	// ("GET /zones?page=2"), which then wins over the bare path.
	responses map[string]string
	requests  []recordedRequest
}

func newFakeAPI(t *testing.T, responses map[string]string) *fakeAPI {
	t.Helper()
	api := &fakeAPI{responses: responses}
	api.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		api.requests = append(api.requests, recordedRequest{r.Method, r.URL.Path, r.URL.RawQuery, r.Header, string(body)})

		key := r.Method + " " + r.URL.Path
		response, ok := api.responses[key+"?"+r.URL.RawQuery]
		if !ok {
			response, ok = api.responses[key]
		}
		if !ok {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		io.WriteString(w, response)
	}))
	t.Cleanup(api.server.Close)
	return api
}

// calls lists the requests received, without query strings.
func (api *fakeAPI) calls() []string {
	var calls []string
	for _, r := range api.requests {
		calls = append(calls, r.method+" "+r.path)
	}
	return calls
}

func newTestCloudflare(t *testing.T, api *fakeAPI) (*Cloudflare, *testLogger) {
	t.Helper()
	logger := newTestLogger(t)
	return NewCloudflare(api.server.URL, "user@example.com", "secret-key", logger.Logger, &logger.stdout), logger
}

func TestListZonesPrintsZoneNamesBetweenLines(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})
	cloudflare, out := newTestCloudflare(t, api)

	if err := cloudflare.listZones(); err != nil {
		t.Fatal(err)
	}

	want := "" +
		"──────────────────────────────\n" +
		"                   example.com\n" +
		"──────────────────────────────\n" +
		"a-much-longer-zone.example.org\n" +
		"──────────────────────────────\n" +
		"\n"
	if out.stdout.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.stdout.String(), want)
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

func TestListZonesFailsWhenAPIRefuses(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": authErrorResponse})
	cloudflare, out := newTestCloudflare(t, api)

	err := cloudflare.listZones()

	if err == nil || !strings.Contains(err.Error(), "Authentication error") {
		t.Errorf("got error %v, want one that carries the API's message", err)
	}
	if out.stdout.String() != "" {
		t.Errorf("got output %q, want none", out.stdout.String())
	}
}

func TestListZonesPrintsBlankLineWhenThereAreNoZones(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": `{"result":[],"success":true}`})
	cloudflare, out := newTestCloudflare(t, api)

	if err := cloudflare.listZones(); err != nil {
		t.Fatal(err)
	}

	if out.stdout.String() != "\n" {
		t.Errorf("got %q, want a single blank line", out.stdout.String())
	}
}

func TestListZonesFailsWhenResponseIsNotJSON(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": "<html>Bad Gateway</html>"})
	cloudflare, _ := newTestCloudflare(t, api)

	if err := cloudflare.listZones(); err == nil {
		t.Error("got no error, want one")
	}
}

func TestListZonesFailsWhenServerIsUnreachable(t *testing.T) {
	api := newFakeAPI(t, nil)
	cloudflare, _ := newTestCloudflare(t, api)
	api.server.Close()

	if err := cloudflare.listZones(); err == nil {
		t.Error("got no error, want one")
	}
}

func TestListsFollowEveryPage(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones?page=1": `{"result":[{"id":"zone-9","name":"other.example"}],` +
			`"result_info":{"page":1,"per_page":1,"count":1,"total_count":2,"total_pages":2},"success":true}`,
		"GET /zones?page=2": `{"result":[{"id":"zone-1","name":"example.com"}],` +
			`"result_info":{"page":2,"per_page":1,"count":1,"total_count":2,"total_pages":2},"success":true}`,
	})
	cloudflare, _ := newTestCloudflare(t, api)

	zoneID, err := cloudflare.getZoneID("example.com")

	if err != nil || zoneID != "zone-1" {
		t.Errorf("got id=%q err=%v, want the zone from the second page", zoneID, err)
	}
	if len(api.requests) != 2 {
		t.Errorf("got %d requests, want 2", len(api.requests))
	}
}

func TestGetZoneIDFailsWhenZoneIsUnknown(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})
	cloudflare, _ := newTestCloudflare(t, api)

	_, err := cloudflare.getZoneID("unknown.example")

	if err == nil || !strings.Contains(err.Error(), "unknown.example") {
		t.Errorf("got error %v, want one that names the zone", err)
	}
}

func TestGetDNSRecordsReturnsEveryRecord(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones/zone-1/dns_records": dnsRecordsResponse})
	cloudflare, _ := newTestCloudflare(t, api)

	records, err := cloudflare.getDNSRecords("zone-1")

	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(records, existingRecords) {
		t.Errorf("got %v, want %v", records, existingRecords)
	}
}

func TestListDNSRecordsByNamePrintsRecordNamesRightAligned(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                    zonesResponse,
		"GET /zones/zone-1/dns_records": dnsRecordsResponse,
	})
	cloudflare, out := newTestCloudflare(t, api)

	if err := cloudflare.listDNSRecordsByName("example.com", false); err != nil {
		t.Fatal(err)
	}

	want := "" +
		"       example.com\n" +
		" music.example.com\n" +
		"       example.com\n" +
		"videos.example.com\n" +
		"\n"
	if out.stdout.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.stdout.String(), want)
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
		"       example.com\n" +
		"──────────────────\n" +
		" music.example.com\n" +
		"──────────────────\n" +
		"       example.com\n" +
		"──────────────────\n" +
		"videos.example.com\n" +
		"──────────────────\n" +
		"\n"
	if out.stdout.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.stdout.String(), want)
	}
}

func TestListDNSRecordsByNameFailsWhenZoneIsUnknown(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})
	cloudflare, out := newTestCloudflare(t, api)

	err := cloudflare.listDNSRecordsByName("unknown.example", false)

	if err == nil {
		t.Error("got no error, want one")
	}
	if out.stdout.String() != "" {
		t.Errorf("got output %q, want none", out.stdout.String())
	}
}

func musicRecord() Param {
	return Param{Type: "A", Name: "music.example.com", Content: "203.0.113.9", TTL: 120, Proxied: false}
}

func TestUpdateRecordPutsWhenAddressChanged(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"PUT /zones/zone-1/dns_records/rec-music": updateResponse})
	cloudflare, _ := newTestCloudflare(t, api)

	outcome, err := cloudflare.updateRecord("zone-1", existingRecords, musicRecord(), false)

	if err != nil || outcome != Updated {
		t.Fatalf("got outcome=%v err=%v, want Updated", outcome, err)
	}
	if !reflect.DeepEqual(api.calls(), []string{"PUT /zones/zone-1/dns_records/rec-music"}) {
		t.Errorf("calls: got %v", api.calls())
	}
}

func TestUpdateRecordSendsRecordAsJSON(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"PUT /zones/zone-1/dns_records/rec-music": updateResponse})
	cloudflare, _ := newTestCloudflare(t, api)
	record := musicRecord()
	record.Proxied = true

	cloudflare.updateRecord("zone-1", existingRecords, record, false)

	var got map[string]any
	if err := json.Unmarshal([]byte(api.requests[0].body), &got); err != nil {
		t.Fatalf("request body is not JSON: %q", api.requests[0].body)
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
	api := newFakeAPI(t, map[string]string{"POST /zones/zone-1/dns_records": updateResponse})
	cloudflare, _ := newTestCloudflare(t, api)
	record := musicRecord()
	record.Name = "new.example.com"

	outcome, err := cloudflare.updateRecord("zone-1", existingRecords, record, false)

	if err != nil || outcome != Updated {
		t.Fatalf("got outcome=%v err=%v, want Updated", outcome, err)
	}
	if !reflect.DeepEqual(api.calls(), []string{"POST /zones/zone-1/dns_records"}) {
		t.Errorf("calls: got %v", api.calls())
	}
}

func TestUpdateRecordPicksTheARecordAmongRecordsOfTheSameName(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"PUT /zones/zone-1/dns_records/rec-apex": updateResponse})
	cloudflare, _ := newTestCloudflare(t, api)
	record := musicRecord()
	record.Name = "example.com" // also the name of the MX record, which comes first

	cloudflare.updateRecord("zone-1", existingRecords, record, false)

	if !reflect.DeepEqual(api.calls(), []string{"PUT /zones/zone-1/dns_records/rec-apex"}) {
		t.Errorf("calls: got %v, want a PUT to the A record", api.calls())
	}
}

func TestUpdateRecordSendsNothingWhenRecordIsUpToDate(t *testing.T) {
	api := newFakeAPI(t, nil)
	cloudflare, out := newTestCloudflare(t, api)
	record := musicRecord()
	record.Name = "videos.example.com" // already at 203.0.113.9

	outcome, err := cloudflare.updateRecord("zone-1", existingRecords, record, false)

	if err != nil || outcome != Unchanged {
		t.Errorf("got outcome=%v err=%v, want Unchanged", outcome, err)
	}
	if len(api.requests) != 0 {
		t.Errorf("got requests %v, want none", api.calls())
	}
	if out.stdout.String() != "" {
		t.Errorf("got output %q, want none", out.stdout.String())
	}
}

func TestUpdateRecordPutsUpToDateRecordWhenForced(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"PUT /zones/zone-1/dns_records/rec-videos": updateResponse})
	cloudflare, _ := newTestCloudflare(t, api)
	record := musicRecord()
	record.Name = "videos.example.com"

	outcome, err := cloudflare.updateRecord("zone-1", existingRecords, record, true)

	if err != nil || outcome != Updated {
		t.Errorf("got outcome=%v err=%v, want Updated", outcome, err)
	}
}

func TestUpdateRecordPutsWhenTTLOrProxyDiffers(t *testing.T) {
	cases := map[string]func(*Param){
		"ttl":   func(p *Param) { p.TTL = 300 },
		"proxy": func(p *Param) { p.Proxied = true },
	}
	for name, change := range cases {
		api := newFakeAPI(t, map[string]string{"PUT /zones/zone-1/dns_records/rec-videos": updateResponse})
		cloudflare, _ := newTestCloudflare(t, api)
		record := musicRecord()
		record.Name = "videos.example.com"
		change(&record)

		outcome, _ := cloudflare.updateRecord("zone-1", existingRecords, record, false)

		if outcome != Updated {
			t.Errorf("%s: got outcome=%v, want Updated", name, outcome)
		}
	}
}

func TestUpdateRecordIgnoresTTLOfProxiedRecord(t *testing.T) {
	// Cloudflare reports ttl 1 ("automatic") for every proxied record.
	records := []Record{{ID: "rec-web", Name: "web.example.com", Type: "A", Content: "203.0.113.9", TTL: 1, Proxied: true}}
	api := newFakeAPI(t, nil)
	cloudflare, _ := newTestCloudflare(t, api)
	record := Param{Type: "A", Name: "web.example.com", Content: "203.0.113.9", TTL: 120, Proxied: true}

	outcome, err := cloudflare.updateRecord("zone-1", records, record, false)

	if err != nil || outcome != Unchanged {
		t.Errorf("got outcome=%v err=%v, want Unchanged", outcome, err)
	}
}

func TestUpdateRecordLogsRawResponseThenPrintsItIndented(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"PUT /zones/zone-1/dns_records/rec-music": updateResponse})
	cloudflare, out := newTestCloudflare(t, api)

	cloudflare.updateRecord("zone-1", existingRecords, musicRecord(), false)

	logLine, printed, _ := strings.Cut(out.stdout.String(), "\n")
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

func TestUpdateRecordFailsWhenAPIRefuses(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"PUT /zones/zone-1/dns_records/rec-music": refusedResponse})
	cloudflare, out := newTestCloudflare(t, api)

	outcome, err := cloudflare.updateRecord("zone-1", existingRecords, musicRecord(), false)

	if err != nil || outcome != Failed {
		t.Errorf("got outcome=%v err=%v, want Failed without an error", outcome, err)
	}
	want := " WARN  music.example.com - Update failed: " + refusedResponse + "\n"
	if !strings.HasSuffix(out.stdout.String(), want) || strings.Count(out.stdout.String(), "\n") != 1 {
		t.Errorf("got %q, want a single line ending in %q", out.stdout.String(), want)
	}
}

func TestUpdateRecordFailsWhenResponseIsNotJSON(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"PUT /zones/zone-1/dns_records/rec-music": "<html>Bad Gateway</html>"})
	cloudflare, _ := newTestCloudflare(t, api)

	_, err := cloudflare.updateRecord("zone-1", existingRecords, musicRecord(), false)

	if err == nil {
		t.Error("got no error, want one")
	}
}

func ipService(t *testing.T, status int, body string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestGetIPReturnsAddress(t *testing.T) {
	for _, body := range []string{"203.0.113.9", "203.0.113.9\n"} {
		ip, err := getIP(ipService(t, http.StatusOK, body))

		if err != nil || ip != "203.0.113.9" {
			t.Errorf("body %q: got ip=%q err=%v", body, ip, err)
		}
	}
}

func TestGetIPRejectsAnythingButAPublicIPv4Address(t *testing.T) {
	cases := map[string]string{
		"error page":       "<html>Too Many Requests</html>",
		"empty":            "",
		"IPv6 address":     "2001:db8::1",
		"private address":  "192.168.0.10",
		"loopback address": "127.0.0.1",
		"unspecified":      "0.0.0.0",
	}
	for name, body := range cases {
		ip, err := getIP(ipService(t, http.StatusOK, body))

		if err == nil {
			t.Errorf("%s: got ip=%q, want an error", name, ip)
		}
	}
}

func TestGetIPFailsOnErrorStatus(t *testing.T) {
	_, err := getIP(ipService(t, http.StatusTooManyRequests, "203.0.113.9"))

	if err == nil {
		t.Error("got no error, want one")
	}
}

func TestGetIPFailsWhenServerIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	if _, err := getIP(server.URL); err == nil {
		t.Error("got no error, want one")
	}
}
