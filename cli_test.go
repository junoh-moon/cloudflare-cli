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

// The help and usage-error texts below were captured from the Kotlin version.
const wantHelp = `Usage: cloudflare [<options>]

Options:
  --email=<email>
  --key=<token>
  --zone=<zone>
  --dns=<dns(,dns)*>
  --proxy
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
	ipService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "203.0.113.9")
	}))
	defer ipService.Close()

	var stdout, stderr bytes.Buffer
	cli := &Cli{
		stdout:   &stdout,
		stderr:   &stderr,
		endpoint: api.server.URL,
		ipURL:    ipService.URL,
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
		"GET /zones":                              zonesResponse,
		"GET /zones/zone-1/dns_records":           dnsRecordsResponse,
		"PUT /zones/zone-1/dns_records/rec-music": updateResponse,
		"PUT /zones/zone-1/dns_records/rec-apex":  updateResponse,
		"POST /zones/zone-1/dns_records":          updateResponse,
	})
}

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
		wantUsage+"Error: missing option --email\nError: missing option --key\n")
}

func TestMissingKeyIsReported(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--email", "a").expect(t, 1, "",
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
	runCli(t, newFakeAPI(t, nil), "--email", "a", "--key", "b", "extra").expect(t, 1, "",
		wantUsage+"Error: got unexpected extra argument (extra)\n")
}

func TestListDNSRequiresZone(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--email", "a", "--key", "b", "--list_dns").expect(t, 1, "",
		"--zone option is required\n")
}

func TestUpdateRequiresDNS(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--email", "a", "--key", "b", "--update", "--zone", "z").expect(t, 1, "",
		"--dns option is required\n")
}

func TestUpdateRequiresZone(t *testing.T) {
	runCli(t, newFakeAPI(t, nil), "--email", "a", "--key", "b", "--update", "--dns", "d").expect(t, 1, "",
		"--zone option is required\n")
}

func TestDoesNothingWithoutModeOption(t *testing.T) {
	api := newFakeAPI(t, nil)

	runCli(t, api, "--email", "a", "--key", "b").expect(t, 0, "", "")

	if len(api.requests) != 0 {
		t.Errorf("got requests %v, want none", api.calls())
	}
}

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
		" music.example.com\n"+
		"       example.com\n"+
		"videos.example.com\n"+
		"\n", "")
}

func TestListZonesWinsOverUpdate(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": zonesResponse})

	runCli(t, api, "--email", "a", "--key", "b", "--update", "--list_zones")

	if !reflect.DeepEqual(api.calls(), []string{"GET /zones"}) {
		t.Errorf("calls: got %v", api.calls())
	}
}

func TestUpdatePointsEveryNameAtPublicIP(t *testing.T) {
	api := newUpdateAPI(t)

	// Same option style as the crontab entry; the empty name is the zone itself.
	run := runCli(t, api, "--update", "--email=a", "--key=b", "--zone=example.com", "--dns=music,,new")

	if run.exitCode != 0 || run.stderr != "" {
		t.Fatalf("got exit=%d stderr=%q", run.exitCode, run.stderr)
	}
	want := []Param{
		{"A", "music.example.com", "203.0.113.9", 120, false},
		{"A", "example.com", "203.0.113.9", 120, false},
		{"A", "new.example.com", "203.0.113.9", 120, false},
	}
	if got := api.sentRecords(t); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUpdateLooksUpZoneAndRecordsForEachName(t *testing.T) {
	api := newUpdateAPI(t)

	runCli(t, api, "--email", "a", "--key", "b", "--update", "--zone", "example.com", "--dns", "music,new")

	want := []string{
		"GET /zones",
		"GET /zones/zone-1/dns_records",
		"PUT /zones/zone-1/dns_records/rec-music",
		"GET /zones",
		"GET /zones/zone-1/dns_records",
		"POST /zones/zone-1/dns_records",
	}
	if !reflect.DeepEqual(api.calls(), want) {
		t.Errorf("calls: got %v, want %v", api.calls(), want)
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

func TestUpdateWritesOneLogLinePerRecord(t *testing.T) {
	api := newUpdateAPI(t)

	run := runCli(t, api, "--email", "a", "--key", "b", "--update", "--zone", "example.com", "--dns", "music,")

	lines := strings.Split(strings.TrimSuffix(readFile(t, run.logPath), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2: %q", len(lines), lines)
	}
	if !strings.HasSuffix(lines[0], " INFO  music.example.com - "+updateResponse) {
		t.Errorf("first line: got %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], " INFO  example.com - "+updateResponse) {
		t.Errorf("second line: got %q", lines[1])
	}
}

func TestUpdateExitsZeroEvenWhenUpdateFails(t *testing.T) {
	api := newFakeAPI(t, map[string]string{"GET /zones": authErrorResponse})

	run := runCli(t, api, "--email", "a", "--key", "b", "--update", "--zone", "example.com", "--dns", "music,videos")

	if run.exitCode != 0 || run.stderr != "" {
		t.Errorf("got exit=%d stderr=%q", run.exitCode, run.stderr)
	}
	if got := strings.Count(run.stdout, "Update failed: zone example.com not found"); got != 2 {
		t.Errorf("got %d warnings, want one per name: %q", got, run.stdout)
	}
}

func TestExitsOneWhenAPIIsUnreachable(t *testing.T) {
	api := newFakeAPI(t, nil)
	api.server.Close()

	run := runCli(t, api, "--email", "a", "--key", "b", "--list_zones")

	if run.exitCode != 1 || !strings.HasPrefix(run.stderr, "Error: ") {
		t.Errorf("got exit=%d stderr=%q", run.exitCode, run.stderr)
	}
}
