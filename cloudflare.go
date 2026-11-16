package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// getIP asks an external service for this machine's public IP address.
func getIP(url string) (string, error) {
	client := &http.Client{Timeout: requestTimeout}
	response, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return string(body), err
}

// listZones prints the name of every zone. It reports false when the
// response carries no result (for example on an authentication error).
func (c *Cloudflare) listZones() (bool, error) {
	body, err := c.get(c.endpoint + "/zones")
	if err != nil {
		return false, err
	}
	zones, err := parseResult(body)
	if err != nil {
		return false, err
	}
	if zones == nil {
		return false, nil
	}

	table := NewTable().addLine()
	for _, zone := range zones {
		table.addRow(zone.Name).addLine()
	}
	fmt.Fprintln(c.out, table.render())
	return true, nil
}

func (c *Cloudflare) listDNSRecordsByName(zoneName string, pretty bool) (bool, error) {
	zoneID, err := c.getZoneID(zoneName)
	if err != nil || zoneID == "" {
		return false, err
	}
	return c.listDNSRecords(zoneID, pretty)
}

// getZoneID returns "" when no zone has the given name.
func (c *Cloudflare) getZoneID(zoneName string) (string, error) {
	body, err := c.get(c.endpoint + "/zones")
	if err != nil {
		return "", err
	}
	return searchID(body, zoneName)
}

func (c *Cloudflare) listDNSRecords(zoneID string, pretty bool) (bool, error) {
	body, err := c.get(c.endpoint + "/zones/" + zoneID + "/dns_records")
	if err != nil {
		return false, err
	}
	records, err := parseResult(body)
	if err != nil {
		return false, err
	}
	if records == nil {
		return false, nil
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
	return true, nil
}

// updateRecord points the record at params.Content, creating the record when
// it does not exist yet. It reports false when the update could not be made.
func (c *Cloudflare) updateRecord(zoneName string, params Param) (bool, error) {
	zoneID, err := c.getZoneID(zoneName)
	if err != nil {
		return false, err
	}
	if zoneID == "" {
		c.logger.warn(params.Name, "Update failed: zone "+zoneName+" not found")
		return false, nil
	}

	records, err := c.get(c.endpoint + "/zones/" + zoneID + "/dns_records")
	if err != nil {
		return false, err
	}
	dnsID, err := c.getDNSID(records, params.Name)
	if err != nil {
		return false, err
	}

	content, err := json.Marshal(params)
	if err != nil {
		return false, err
	}

	var response string
	if dnsID == "" {
		response, err = c.request(http.MethodPost, c.endpoint+"/zones/"+zoneID+"/dns_records", content)
	} else {
		response, err = c.request(http.MethodPut, c.endpoint+"/zones/"+zoneID+"/dns_records/"+dnsID, content)
	}
	if err != nil {
		return false, err
	}
	c.logger.info(params.Name, response)

	if strings.TrimSpace(response) == "" {
		c.logger.warn(params.Name, "Update failed: empty response")
		return false, nil
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, []byte(response), "", "  "); err != nil {
		return false, fmt.Errorf("unexpected response %q: %w", response, err)
	}
	fmt.Fprintln(c.out, indented.String())
	return true, nil
}

func (c *Cloudflare) getDNSID(body string, dnsName string) (string, error) {
	return searchID(body, dnsName)
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

// entry is one item of a list response: a zone or a DNS record.
type entry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// parseResult extracts the "result" list of a response. The list is nil when
// the response has no result, which is how the API reports an error.
func parseResult(body string) ([]entry, error) {
	var response struct {
		Result []entry `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return nil, fmt.Errorf("unexpected response %q: %w", body, err)
	}
	return response.Result, nil
}

// searchID returns the id of the entry with the given name, or "" if absent.
func searchID(body string, name string) (string, error) {
	entries, err := parseResult(body)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Name == name {
			return e.ID, nil
		}
	}
	return "", nil
}
