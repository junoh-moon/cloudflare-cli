package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/pflag"
)

const usage = "Usage: cloudflare [<options>]\n"

const help = usage + `
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

// Cli parses the command line and runs one of: list zones, list DNS records,
// update DNS records.
type Cli struct {
	stdout   io.Writer
	stderr   io.Writer
	endpoint string // Cloudflare API
	ipURL    string // service that answers with this machine's public IP
	logPath  string
}

func NewCli() *Cli {
	home, _ := os.UserHomeDir()
	return &Cli{
		stdout:   os.Stdout,
		stderr:   os.Stderr,
		endpoint: "https://api.cloudflare.com/client/v4",
		ipURL:    "https://ipinfo.io/ip",
		logPath:  filepath.Join(home, ".dns.log"),
	}
}

// run returns the process exit code: 0 when everything asked for was done.
func (c *Cli) run(args []string) int {
	flags := pflag.NewFlagSet("cloudflare", pflag.ContinueOnError)
	flags.SetOutput(io.Discard) // errors are printed below, in our own format
	email := flags.String("email", "", "")
	key := flags.String("key", "", "")
	zoneName := flags.String("zone", "", "")
	dns := flags.String("dns", "", "")
	useProxy := flags.Bool("proxy", false, "")
	force := flags.Bool("force", false, "")
	listZones := flags.Bool("list_zones", false, "")
	listDNSRecords := flags.Bool("list_dns", false, "")
	update := flags.Bool("update", false, "")
	showHelp := flags.BoolP("help", "h", false, "")

	if err := flags.Parse(args); err != nil {
		return c.usageError(err.Error())
	}
	if *showHelp {
		fmt.Fprint(c.stdout, help)
		return 0
	}
	if flags.NArg() > 0 {
		return c.usageError("got unexpected extra argument (" + strings.Join(flags.Args(), " ") + ")")
	}

	modes := 0
	for _, chosen := range []bool{*listZones, *listDNSRecords, *update} {
		if chosen {
			modes++
		}
	}
	if modes == 0 {
		return c.usageError("one of --list_zones, --list_dns, --update is required")
	}
	if modes > 1 {
		return c.usageError("only one of --list_zones, --list_dns, --update may be given")
	}

	// Which options the chosen mode needs; --dns may be empty, the others may not.
	required := []string{"email", "key"}
	switch {
	case *listDNSRecords:
		required = append(required, "zone")
	case *update:
		required = append(required, "zone", "dns")
	}
	var problems []string
	for _, name := range required {
		if !flags.Changed(name) {
			problems = append(problems, "missing option --"+name)
		} else if name != "dns" && flags.Lookup(name).Value.String() == "" {
			problems = append(problems, "option --"+name+" must not be empty")
		}
	}
	if len(problems) > 0 {
		return c.usageError(problems...)
	}

	logger := NewLogger(c.stdout, c.stderr, c.logPath)
	defer logger.close()
	cloudflare := NewCloudflare(c.endpoint, *email, *key, logger, c.stdout)

	var err error
	switch {
	case *listZones:
		err = cloudflare.listZones()
	case *listDNSRecords:
		err = cloudflare.listDNSRecordsByName(*zoneName, false)
	case *update:
		err = c.update(cloudflare, logger, *zoneName, strings.Split(*dns, ","), *useProxy, *force)
	}
	if err != nil {
		subject := *zoneName
		if subject == "" {
			subject = "cloudflare"
		}
		logger.error(subject, err.Error())
		return 1
	}
	return 0
}

// update points every name in dnsList at this machine's public IP.
// An empty name stands for the zone itself.
func (c *Cli) update(cloudflare *Cloudflare, logger *Logger, zoneName string, dnsList []string, useProxy, force bool) error {
	ip, err := getIP(c.ipURL)
	if err != nil {
		return err
	}
	zoneID, err := cloudflare.getZoneID(zoneName)
	if err != nil {
		return err
	}
	records, err := cloudflare.getDNSRecords(zoneID)
	if err != nil {
		return err
	}

	count := map[Outcome]int{}
	for _, name := range dnsList {
		dnsName := zoneName
		if strings.TrimSpace(name) != "" {
			dnsName = name + "." + zoneName
		}
		proxied := useProxy && forWeb(dnsName, zoneName)
		record := Param{Type: "A", Name: dnsName, Content: ip, TTL: recordTTL, Proxied: proxied}

		// A refused record is logged and counted; the remaining names still get their turn.
		outcome, err := cloudflare.updateRecord(zoneID, records, record, force)
		if err != nil {
			return err
		}
		count[outcome]++
	}

	summary := fmt.Sprintf("%d names at %s: %d updated, %d unchanged, %d failed",
		len(dnsList), ip, count[Updated], count[Unchanged], count[Failed])
	if count[Failed] > 0 {
		return fmt.Errorf("%s", summary)
	}
	logger.info(zoneName, summary)
	return nil
}

// forWeb tells whether a record serves web traffic and so may sit behind
// Cloudflare's proxy. The zone itself and unproxiedSubdomains may not.
func forWeb(dnsName, zoneName string) bool {
	if dnsName == zoneName {
		return false
	}
	return !slices.ContainsFunc(unproxiedSubdomains, func(subdomain string) bool {
		return dnsName == subdomain+"."+zoneName
	})
}

func (c *Cli) usageError(messages ...string) int {
	fmt.Fprint(c.stderr, usage+"\n")
	for _, message := range messages {
		fmt.Fprintln(c.stderr, "Error: "+message)
	}
	return 1
}

func main() {
	os.Exit(NewCli().run(os.Args[1:]))
}
