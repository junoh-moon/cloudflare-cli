package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const requestTimeout = 30 * time.Second

// Cloudflare is a client for the Cloudflare API v4.
type Cloudflare struct {
	client   *http.Client
	endpoint string
	header   map[string]string
	logger   *Logger
	out      io.Writer
}

func NewCloudflare(endpoint, email, key string, logger *Logger, out io.Writer) *Cloudflare {
	return &Cloudflare{
		client:   &http.Client{Timeout: requestTimeout},
		endpoint: endpoint,
		header: map[string]string{
			"X-Auth-Email": email,
			"X-Auth-Key":   key,
			"Content-Type": "application/json",
		},
		logger: logger,
		out:    out,
	}
}

type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Record is a DNS record as the API reports it.
type Record struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// matches tells whether the record already holds what params would write.
func (r Record) matches(params Param) bool {
	// Cloudflare reports ttl 1 ("automatic") for every proxied record,
	// whatever TTL was sent.
	sameTTL := r.TTL == params.TTL || params.Proxied
	return r.Content == params.Content && r.Proxied == params.Proxied && sameTTL
}

// Outcome is what updateRecord did with one record.
type Outcome int

const (
	Updated   Outcome = iota // written, and accepted by the API
	Unchanged                // already up to date, nothing sent
	Failed                   // written, but refused by the API
)

// getIP asks an external service for this machine's public IPv4 address.
func getIP(url string) (string, error) {
	client := &http.Client{Timeout: requestTimeout}
	response, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IP lookup at %s answered %s", url, response.Status)
	}

	text := strings.TrimSpace(string(body))
	address, err := netip.ParseAddr(text)
	if err != nil {
		return "", fmt.Errorf("IP lookup at %s answered %q, which is not an IP address", url, text)
	}
	isPublic := !address.IsPrivate() && !address.IsLoopback() && !address.IsUnspecified() &&
		!address.IsLinkLocalUnicast() && !address.IsMulticast()
	if !address.Is4() || !isPublic {
		return "", fmt.Errorf("IP lookup at %s answered %s, which is not a public IPv4 address", url, address)
	}
	return address.String(), nil
}

// listZones prints the name of every zone.
func (c *Cloudflare) listZones() error {
	zones, err := getAll[Zone](c, c.endpoint+"/zones")
	if err != nil {
		return err
	}

	table := NewTable().addLine()
	for _, zone := range zones {
		table.addRow(zone.Name).addLine()
	}
	fmt.Fprintln(c.out, table.render())
	return nil
}

func (c *Cloudflare) listDNSRecordsByName(zoneName string, pretty bool) error {
	zoneID, err := c.getZoneID(zoneName)
	if err != nil {
		return err
	}
	return c.listDNSRecords(zoneID, pretty)
}

func (c *Cloudflare) getZoneID(zoneName string) (string, error) {
	zones, err := getAll[Zone](c, c.endpoint+"/zones")
	if err != nil {
		return "", err
	}
	for _, zone := range zones {
		if zone.Name == zoneName {
			return zone.ID, nil
		}
	}
	return "", fmt.Errorf("zone %s not found", zoneName)
}

// listDNSRecords prints the name of every record in the zone.
func (c *Cloudflare) listDNSRecords(zoneID string, pretty bool) error {
	records, err := c.getDNSRecords(zoneID)
	if err != nil {
		return err
	}

	table := NewTable()
	if pretty {
		table.addLine()
	}
	for _, record := range records {
		table.addRow(record.Name)
		if pretty {
			table.addLine()
		}
	}
	fmt.Fprintln(c.out, table.render())
	return nil
}

func (c *Cloudflare) getDNSRecords(zoneID string) ([]Record, error) {
	return getAll[Record](c, c.endpoint+"/zones/"+zoneID+"/dns_records")
}

// updateRecord makes the zone hold params, given the records it has now:
// it creates the record when there is none of that name and type, and leaves
// an up-to-date record alone unless force is set.
//
// A write the API refuses is logged and reported as Failed, not as an error;
// an error means the API could not be reached or understood.
func (c *Cloudflare) updateRecord(zoneID string, records []Record, params Param, force bool) (Outcome, error) {
	existing := findRecord(records, params.Name, params.Type)
	if existing != nil && existing.matches(params) && !force {
		return Unchanged, nil
	}

	content, err := json.Marshal(params)
	if err != nil {
		return Failed, err
	}
	var response string
	if existing == nil {
		response, err = c.request(http.MethodPost, c.endpoint+"/zones/"+zoneID+"/dns_records", content)
	} else {
		response, err = c.request(http.MethodPut, c.endpoint+"/zones/"+zoneID+"/dns_records/"+existing.ID, content)
	}
	if err != nil {
		return Failed, err
	}

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return Failed, fmt.Errorf("unexpected response %q: %w", response, err)
	}
	if !result.Success {
		c.logger.warn(params.Name, "Update failed: "+response)
		return Failed, nil
	}

	c.logger.info(params.Name, response)
	var indented bytes.Buffer
	if err := json.Indent(&indented, []byte(response), "", "  "); err != nil {
		return Failed, err
	}
	fmt.Fprintln(c.out, indented.String())
	return Updated, nil
}

// findRecord returns the record of the given name and type, or nil.
// Several records may share a name: a zone often has both A and MX records.
func findRecord(records []Record, name, recordType string) *Record {
	for i := range records {
		if records[i].Name == name && records[i].Type == recordType {
			return &records[i]
		}
	}
	return nil
}

// getAll fetches a list endpoint page by page and returns every item.
func getAll[T any](c *Cloudflare, url string) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		body, err := c.get(fmt.Sprintf("%s?page=%d", url, page))
		if err != nil {
			return nil, err
		}

		var response struct {
			Success    bool `json:"success"`
			Result     []T  `json:"result"`
			ResultInfo struct {
				TotalPages int `json:"total_pages"`
			} `json:"result_info"`
		}
		if err := json.Unmarshal([]byte(body), &response); err != nil {
			return nil, fmt.Errorf("unexpected response %q: %w", body, err)
		}
		if !response.Success {
			return nil, fmt.Errorf("the API refused the request: %s", body)
		}

		all = append(all, response.Result...)
		if page >= response.ResultInfo.TotalPages {
			return all, nil
		}
	}
}

func (c *Cloudflare) get(url string) (string, error) {
	return c.request(http.MethodGet, url, nil)
}

func (c *Cloudflare) request(method, url string, body []byte) (string, error) {
	request, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	for name, value := range c.header {
		request.Header.Set(name, value)
	}

	response, err := c.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	return string(responseBody), err
}
