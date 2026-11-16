package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		ipURL:    "http://ipinfo.io/ip",
		logPath:  filepath.Join(home, ".dns.log"),
	}
}

// run returns the process exit code.
func (c *Cli) run(args []string) int {
	flags := pflag.NewFlagSet("cloudflare", pflag.ContinueOnError)
	flags.SetOutput(io.Discard) // errors are printed below, in our own format
	email := flags.String("email", "", "")
	key := flags.String("key", "", "")
	zoneName := flags.String("zone", "", "")
	dns := flags.String("dns", "", "")
	useProxy := flags.Bool("proxy", false, "")
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
	var missing []string
	for _, name := range []string{"email", "key"} {
		if !flags.Changed(name) {
			missing = append(missing, "missing option --"+name)
		}
	}
	if len(missing) > 0 {
		return c.usageError(missing...)
	}

	logger := NewLogger(c.stdout, c.logPath)
	defer logger.close()
	cloudflare := NewCloudflare(c.endpoint, *email, *key, logger, c.stdout)

	var err error
	switch {
	case *listZones:
		_, err = cloudflare.listZones()
	case *listDNSRecords:
		if !flags.Changed("zone") {
			return c.fail("--zone option is required")
		}
		_, err = cloudflare.listDNSRecordsByName(*zoneName, false)
	case *update:
		if !flags.Changed("dns") {
			return c.fail("--dns option is required")
		}
		if !flags.Changed("zone") {
			return c.fail("--zone option is required")
		}
		err = c.update(cloudflare, *zoneName, strings.Split(*dns, ","), *useProxy)
	}
	if err != nil {
		return c.fail("Error: " + err.Error())
	}
	return 0
}

// update points every name in dnsList at this machine's public IP.
// An empty name stands for the zone itself.
func (c *Cli) update(cloudflare *Cloudflare, zoneName string, dnsList []string, useProxy bool) error {
	ip, err := getIP(c.ipURL)
	if err != nil {
		return err
	}

	for _, name := range dnsList {
		dnsName := zoneName
		if strings.TrimSpace(name) != "" {
			dnsName = name + "." + zoneName
		}
		proxied := useProxy && forWeb(dnsName, zoneName)
		record := Param{Type: "A", Name: dnsName, Content: ip, TTL: 120, Proxied: proxied}

		// A failed update is already logged; the remaining names still get their turn.
		if _, err := cloudflare.updateRecord(zoneName, record); err != nil {
			return err
		}
	}
	return nil
}

// forWeb tells whether a record serves web traffic and so may sit behind
// Cloudflare's proxy. The zone itself and the hub subdomain may not.
func forWeb(dnsName, zoneName string) bool {
	return dnsName != zoneName && dnsName != "hub."+zoneName
}

func (c *Cli) usageError(messages ...string) int {
	fmt.Fprint(c.stderr, usage+"\n")
	for _, message := range messages {
		fmt.Fprintln(c.stderr, "Error: "+message)
	}
	return 1
}

func (c *Cli) fail(message string) int {
	fmt.Fprintln(c.stderr, message)
	return 1
}

func main() {
	os.Exit(NewCli().run(os.Args[1:]))
}
