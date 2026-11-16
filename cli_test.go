package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const wantHelp = `Usage: cloudflare [<options>]

Options:
  --email=<email>
  --key=<token>
  --zone=<zone>
  --dns=<dns(,dns)*>
  --proxy
  --force
  --list_zones
  --list_dns
  --update
  -h, --help          Show this message and exit
`

const wantUsage = "Usage: cloudflare [<options>]\n\n"

type cliRun struct {
	exitCode int
	stdout   string
	stderr   string
	logPath  string
}

// runCli runs the CLI against the fake API and a fake "what is my IP"
// service that always answers 203.0.113.9.
func runCli(t *testing.T, api *fakeAPI, args ...string) cliRun {
	t.Helper()
	return runCliWithIPService(t, api, ipService(t, http.StatusOK, "203.0.113.9"), args...)
}

func runCliWithIPService(t *testing.T, api *fakeAPI, ipURL string, args ...string) cliRun {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cli := &Cli{
		stdout:   &stdout,
		stderr:   &stderr,
		endpoint: api.server.URL,
		ipURL:    ipURL,
		logPath:  filepath.Join(t.TempDir(), ".dns.log"),
	}
	exitCode := cli.run(args)
	return cliRun{exitCode, stdout.String(), stderr.String(), cli.logPath}
}

func (r cliRun) expect(t *testing.T, exitCode int, stdout, stderr string) {
	t.Helper()
	if r.exitCode != exitCode {
		t.Errorf("exit code: got %d, want %d", r.exitCode, exitCode)
	}
	if r.stdout != stdout {
		t.Errorf("stdout: got %q, want %q", r.stdout, stdout)
	}
	if r.stderr != stderr {
		t.Errorf("stderr: got %q, want %q", r.stderr, stderr)
	}
}

// expectErrorLine checks for exit code 1 and a single ERROR log line, on
// stderr and in the log file, that ends with the given text.
func (r cliRun) expectErrorLine(t *testing.T, suffix string) {
	t.Helper()
	if r.exitCode != 1 {
		t.Errorf("exit code: got %d, want 1", r.exitCode)
	}
	want := regexp.MustCompile(`^` + timestampPattern + ` ERROR .*` + regexp.QuoteMeta(suffix) + `\n$`)
	if !want.MatchString(r.stderr) {
		t.Errorf("stderr: got %q, want one ERROR line ending in %q", r.stderr, suffix)
	}
	if log := readFile(t, r.logPath); !strings.HasSuffix(log, r.stderr) {
		t.Errorf("log file: got %q, want it to end with the stderr line", log)
	}
}

// sentRecords returns the record bodies of all PUT and POST requests.
func (api *fakeAPI) sentRecords(t *testing.T) []Param {
	t.Helper()
	var records []Param
	for _, request := range api.requests {
		if request.method == http.MethodGet {
			continue
		}
		var record Param
		if err := json.Unmarshal([]byte(request.body), &record); err != nil {
			t.Fatalf("request body is not a record: %q", request.body)
		}
		records = append(records, record)
	}
	return records
}

func newUpdateAPI(t *testing.T) *fakeAPI {
	return newFakeAPI(t, map[string]string{
		"GET /zones":                               zonesResponse,
		"GET /zones/zone-1/dns_records":            dnsRecordsResponse,
		"PUT /zones/zone-1/dns_records/rec-music":  updateResponse,
		"PUT /zones/zone-1/dns_records/rec-apex":   updateResponse,
		"PUT /zones/zone-1/dns_records/rec-videos": updateResponse,
		"POST /zones/zone-1/dns_records":           updateResponse,
	})
}

// --- options ---

func TestHelpPrintsUsage(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		runCli(t, newFakeAPI(t, nil), flag).expect(t, 0, wantHelp, "")
	}
}

func TestHelpWinsOverOtherOptions(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--update", "--help").expect(t, 0, wantHelp, "")
}

func TestMissingRequiredOptionsAreReportedTogether(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--update").expect(t, 1, "",
		wantUsage+"Error: missing option --email\nError: missing option --key\n"+
			"Error: missing option --zone\nError: missing option --dns\n")
}

func TestMissingKeyIsReported(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--email", "a", "--list_zones").expect(t, 1, "",
		wantUsage+"Error: missing option --key\n")
}

func TestUnknownOptionIsRejected(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--email", "a", "--key", "b", "--bogus").expect(t, 1, "",
		wantUsage+"Error: unknown flag: --bogus\n")
}

func TestLongOptionWithSingleDashIsRejected(t *testing.T) {
	run := runCli(t, newFakeAPI(t, nil), "-email", "a", "--key", "b", "--list_zones")

	if run.exitCode != 1 || !strings.HasPrefix(run.stderr, wantUsage+"Error: unknown shorthand flag") {
		t.Errorf("got exit=%d stderr=%q", run.exitCode, run.stderr)
	}
}

func TestExtraArgumentIsRejected(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--email", "a", "--key", "b", "--list_zones", "extra").expect(t, 1, "",
		wantUsage+"Error: got unexpected extra argument (extra)\n")
}

func TestEmptyValuesAreRejected(t *testing.T) {
	cases := map[string][]string{
		"email": {"--email", "", "--key", "b", "--list_zones"},
		"key":   {"--email", "a", "--key", "", "--list_zones"},
		"zone":  {"--email", "a", "--key", "b", "--list_dns", "--zone", ""},
	}
	for option, args := range cases {
		api := newFakeAPI(t, nil)

		runCli(t, api, args...).expect(t, 1, "",
			wantUsage+"Error: option --"+option+" must not be empty\n")

		if len(api.requests) != 0 {
			t.Errorf("%s: got requests %v, want none", option, api.calls())
		}
	}
}

func TestModeOptionIsRequired(t *testing.T) {
	api := newFakeAPI(t, nil)

	runCli(t, api, "--email", "a", "--key", "b").expect(t, 1, "",
		wantUsage+"Error: one of --list_zones, --list_dns, --update is required\n")

	if len(api.requests) != 0 {
		t.Errorf("got requests %v, want none", api.calls())
	}
}

func TestOnlyOneModeOptionIsAllowed(t *testing.T) {
	api := newFakeAPI(t, nil)

	runCli(t, api, "--email", "a", "--key", "b", "--update", "--list_zones").expect(t, 1, "",
		wantUsage+"Error: only one of --list_zones, --list_dns, --update may be given\n")

	if len(api.requests) != 0 {
		t.Errorf("got requests %v, want none", api.calls())
	}
}

func TestListDNSRequiresZone(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--email", "a", "--key", "b", "--list_dns").expect(t, 1, "",
		wantUsage+"Error: missing option --zone\n")
}

func TestUpdateRequiresDNSAndZone(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--email", "a", "--key", "b", "--update").expect(t, 1, "",
		wantUsage+"Error: missing option --zone\nError: missing option --dns\n")
}

// --- list modes ---

func TestListZones(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})

	run := runCli(t, api, "--email=user@example.com", "--key=secret-key", "--list_zones")

	run.expect(t, 0, ""+
		"──────────────────────────────\n"+
		"                   example.com\n"+
		"──────────────────────────────\n"+
		"a-much-longer-zone.example.org\n"+
		"──────────────────────────────\n"+
		"\n", "")
	if got := api.requests[0].header.Get("X-Auth-Key"); got != "secret-key" {
		t.Errorf("X-Auth-Key: got %q", got)
	}
}

func TestListDNS(t *testing.T) {
	api := newFakeAPI(t, map[string]string{
		"GET /zones":                    zonesResponse,
		"GET /zones/zone-1/dns_records": dnsRecordsResponse,
	})

	run := runCli(t, api, "--email", "a", "--key", "b", "--list_dns", "--zone", "example.com")

	run.expect(t, 0, ""+
		"       example.com\n"+
		" music.example.com\n"+
		"       example.com\n"+
		"videos.example.com\n"+
		"\n", "")
}

func TestListZonesExitsOneWhenAPIRefuses(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": authErrorResponse})

	run := runCli(t, api, "--email", "a", "--key", "b", "--list_zones")

	run.expectErrorLine(t, authErrorResponse)
	if run.stdout != "" {
		t.Errorf("stdout: got %q, want nothing", run.stdout)
	}
}

func TestListDNSFailsWhenZoneIsUnknown(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})

	run := runCli(t, api, "--email", "a", "--key", "b", "--list_dns", "--zone", "unknown.example")

	run.expectErrorLine(t, "unknown.example - zone unknown.example not found")
}

func TestFailsWhenAPIIsUnreachable(t *testing.T) {
	api := newFakeAPI(t, nil)
	api.server.Close()

	run := runCli(t, api, "--email", "a", "--key", "b", "--list_zones")

	if run.exitCode != 1 || !strings.Contains(run.stderr, " ERROR cloudflare - ") {
		t.Errorf("got exit=%d stderr=%q", run.exitCode, run.stderr)
	}
}

// --- update mode ---

func TestUpdatePointsStaleNamesAtPublicIP(t *testing.T) {
	api := newUpdateAPI(t)

	// Same option style as the crontab entry; the empty name is the zone itself.
	run := runCli(t, api, "--update", "--email=a", "--key=b", "--zone=example.com", "--dns=music,,videos,new")

	if run.exitCode != 0 || run.stderr != "" {
		t.Fatalf("got exit=%d stderr=%q", run.exitCode, run.stderr)
	}
	want := []Param{
		{"A", "music.example.com", "203.0.113.9", 120, false},
		{"A", "example.com", "203.0.113.9", 120, false},
		{"A", "new.example.com", "203.0.113.9", 120, false},
	}
	if got := api.sentRecords(t); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (videos is already up to date)", got, want)
	}
}

func TestUpdateLooksUpZoneAndRecordsOnce(t *testing.T) {
	api := newUpdateAPI(t)

	runCli(t, api, "--email", "a", "--key", "b", "--update", "--zone", "example.com", "--dns", "music,,new")

	want := []string{
		"GET /zones",
		"GET /zones/zone-1/dns_records",
		"PUT /zones/zone-1/dns_records/rec-music",
		"PUT /zones/zone-1/dns_records/rec-apex",
		"POST /zones/zone-1/dns_records",
	}
	if !reflect.DeepEqual(api.calls(), want) {
		t.Errorf("calls: got %v, want %v", api.calls(), want)
	}
}

func TestUpdateEndsWithSummaryLine(t *testing.T) {
	api := newUpdateAPI(t)

	run := runCli(t, api, "--email", "a", "--key", "b", "--update", "--zone", "example.com", "--dns", "music,,videos,new")

	lines := strings.Split(strings.TrimSuffix(readFile(t, run.logPath), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d log lines, want one per updated record and a summary: %q", len(lines), lines)
	}
	if !strings.HasSuffix(lines[0], " INFO  music.example.com - "+updateResponse) {
		t.Errorf("first line: got %q", lines[0])
	}
	if !strings.HasSuffix(lines[3], " INFO  example.com - 4 names at 203.0.113.9: 3 updated, 1 unchanged, 0 failed") {
		t.Errorf("summary: got %q", lines[3])
	}
}

func TestUpdateWritesOnlySummaryWhenNothingChanged(t *testing.T) {
	api := newUpdateAPI(t)

	run := runCli(t, api, "--email", "a", "--key", "b", "--update", "--zone", "example.com", "--dns", "videos")

	if run.exitCode != 0 || run.stderr != "" {
		t.Fatalf("got exit=%d stderr=%q", run.exitCode, run.stderr)
	}
	want := regexp.MustCompile(`^` + timestampPattern + ` INFO  example\.com - 1 names at 203\.0\.113\.9: 0 updated, 1 unchanged, 0 failed\n$`)
	if !want.MatchString(run.stdout) {
		t.Errorf("stdout: got %q", run.stdout)
	}
	if got := api.sentRecords(t); len(got) != 0 {
		t.Errorf("got writes %v, want none", got)
	}
}

func TestUpdateWithForceWritesUpToDateRecords(t *testing.T) {
	api := newUpdateAPI(t)

	run := runCli(t, api, "--email", "a", "--key", "b", "--update", "--force", "--zone", "example.com", "--dns", "videos")

	if got := api.sentRecords(t); len(got) != 1 || got[0].Name != "videos.example.com" {
		t.Errorf("got writes %v, want videos", got)
	}
	if !strings.Contains(run.stdout, "1 names at 203.0.113.9: 1 updated, 0 unchanged, 0 failed") {
		t.Errorf("stdout: got %q", run.stdout)
	}
}

func TestUpdateTreatsBlankNameAsZone(t *testing.T) {
	api := newUpdateAPI(t)

	runCli(t, api, "--email", "a", "--key", "b", "--update", "--zone", "example.com", "--dns", " ")

	if got := api.sentRecords(t); len(got) != 1 || got[0].Name != "example.com" {
		t.Errorf("got %v", got)
	}
}

func TestUpdateWithProxyProxiesAllButZoneAndHub(t *testing.T) {
	api := newUpdateAPI(t)

	runCli(t, api, "--email", "a", "--key", "b", "--update", "--proxy", "--zone", "example.com", "--dns", "music,,hub")

	proxied := map[string]bool{}
	for _, record := range api.sentRecords(t) {
		proxied[record.Name] = record.Proxied
	}
	want := map[string]bool{
		"music.example.com": true,
		"example.com":       false,
		"hub.example.com":   false,
	}
	if !reflect.DeepEqual(proxied, want) {
		t.Errorf("got %v, want %v", proxied, want)
	}
}

func TestUpdateFailsWhenAPIRefusesARecordButStillUpdatesTheRest(t *testing.T) {
	api := newUpdateAPI(t)
	api.responses["PUT /zones/zone-1/dns_records/rec-music"] = refusedResponse

	run := runCli(t, api, "--email", "a", "--key", "b", "--update", "--zone", "example.com", "--dns", "music,videos,new")

	run.expectErrorLine(t, "example.com - 3 names at 203.0.113.9: 1 updated, 1 unchanged, 1 failed")
	if !strings.Contains(run.stdout, " WARN  music.example.com - Update failed: "+refusedResponse) {
		t.Errorf("stdout: got %q, want a warning for music", run.stdout)
	}
	if got := api.calls()[len(api.calls())-1]; got != "POST /zones/zone-1/dns_records" {
		t.Errorf("last call: got %q, want the record after the failed one to be written", got)
	}
}

func TestUpdateFailsBeforeTouchingAnythingWhenIPLookupFails(t *testing.T) {
	api := newUpdateAPI(t)
	badIPService := ipService(t, http.StatusOK, "<html>Too Many Requests</html>")

	run := runCliWithIPService(t, api, badIPService,
		"--email", "a", "--key", "b", "--update", "--zone", "example.com", "--dns", "music")

	if run.exitCode != 1 || !strings.Contains(run.stderr, " ERROR example.com - ") {
		t.Errorf("got exit=%d stderr=%q", run.exitCode, run.stderr)
	}
	if len(api.requests) != 0 {
		t.Errorf("got requests %v, want none", api.calls())
	}
}

func TestUpdateFailsWhenZoneIsUnknown(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})

	run := runCli(t, api, "--email", "a", "--key", "b", "--update", "--zone", "unknown.example", "--dns", "music")

	run.expectErrorLine(t, "unknown.example - zone unknown.example not found")
}
