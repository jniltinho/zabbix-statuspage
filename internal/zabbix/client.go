package zabbix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Tag struct {
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

type Host struct {
	HostID      string `json:"hostid"`
	Host        string `json:"host"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type DiscoveredHost struct {
	HostID      string `json:"hostid"`
	Host        string `json:"host"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DisplayName returns the visible name, falling back to the technical host name.
func (h DiscoveredHost) DisplayName() string {
	if h.Name != "" {
		return h.Name
	}
	return h.Host
}

type HostGroup struct {
	GroupID string `json:"groupid"`
}

type LastEvent struct {
	Name  string `json:"name"`
	Clock string `json:"clock"`
	Value string `json:"value"`
}

// UnmarshalJSON handles Zabbix returning [] when there is no last event.
func (le *LastEvent) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "[]" || s == "null" || s == "" {
		return nil
	}
	type alias LastEvent
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*le = LastEvent(a)
	return nil
}

type Trigger struct {
	TriggerID   string      `json:"triggerid"`
	Description string      `json:"description"`
	Priority    string      `json:"priority"`
	Status      string      `json:"status"`
	Value       string      `json:"value"`
	LastEvent   LastEvent   `json:"lastEvent"`
	Hosts       []Host      `json:"hosts"`
	HostGroups  []HostGroup `json:"hostgroups"`
}

type Event struct {
	EventID  string `json:"eventid"`
	REventID string `json:"r_eventid"`
	Clock    string `json:"clock"`
	Value    string `json:"value"`
	Severity string `json:"severity"`
	Name     string `json:"name"`
	Hosts    []Host `json:"hosts"`
}

type TimePeriod struct {
	PeriodType string `json:"timeperiod_type"`
}

type Maintenance struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	ActiveSince string       `json:"active_since"`
	ActiveTill  string       `json:"active_till"`
	TimePeriods []TimePeriod `json:"timeperiods"`
}

type cacheEntry struct {
	data      any
	expiresAt time.Time
}

type Client struct {
	httpClient *http.Client
	apiURL     string
	apiToken   string
	cacheTTL   time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

func New(apiURL, apiToken string, cacheTTLSeconds int) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiURL:     apiURL,
		apiToken:   apiToken,
		cacheTTL:   time.Duration(cacheTTLSeconds) * time.Second,
		cache:      make(map[string]cacheEntry),
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      int    `json:"id"`
}

type rpcError struct {
	Message string `json:"message"`
	Data    string `json:"data"`
}

type rpcResponse struct {
	Error  *rpcError       `json:"error"`
	Result json.RawMessage `json:"result"`
}

func (c *Client) call(method string, params any, result any) error {
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, c.apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("zabbix http error: status %d", resp.StatusCode)
	}

	var rpc rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		return err
	}
	if rpc.Error != nil {
		return fmt.Errorf("zabbix api error: %s %s", rpc.Error.Message, rpc.Error.Data)
	}

	return json.Unmarshal(rpc.Result, result)
}

func (c *Client) fromCache(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.data, true
}

func (c *Client) setCache(key string, data any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cacheEntry{data: data, expiresAt: time.Now().Add(c.cacheTTL)}
}

// FetchHostsByTags discovers hosts via host-level tags (host.get).
func (c *Client) FetchHostsByTags(tags []Tag) ([]DiscoveredHost, error) {
	const key = "hosts_by_tags"
	if v, ok := c.fromCache(key); ok {
		return v.([]DiscoveredHost), nil
	}

	params := map[string]any{
		"tags":   tags,
		"output": []string{"hostid", "host", "name", "description"},
	}

	var result []DiscoveredHost
	if err := c.call("host.get", params, &result); err != nil {
		return nil, err
	}

	c.setCache(key, result)
	return result, nil
}

// FetchTriggersByHostIDs returns all enabled triggers for the given host IDs.
func (c *Client) FetchTriggersByHostIDs(hostIDs []string) ([]Trigger, error) {
	key := fmt.Sprintf("triggers_hosts:%v", hostIDs)
	if v, ok := c.fromCache(key); ok {
		return v.([]Trigger), nil
	}

	params := map[string]any{
		"hostids":           hostIDs,
		"maintenance":       false,
		"expandDescription": true,
		"output":            []string{"triggerid", "description", "priority", "status", "value"},
		"selectHosts":       []string{"host", "name", "description"},
		"selectHostGroups":  []string{"groupid"},
		"selectLastEvent":   []string{"name", "clock", "value"},
		"filter":            map[string]any{"status": "0"},
	}

	var result []Trigger
	if err := c.call("trigger.get", params, &result); err != nil {
		return nil, err
	}

	c.setCache(key, result)
	return result, nil
}

// FetchEventsByHostIDs returns problem events for the given host IDs.
func (c *Client) FetchEventsByHostIDs(hostIDs []string, timeFrom time.Time) ([]Event, error) {
	key := fmt.Sprintf("events_hosts:%v:%d", hostIDs, timeFrom.Unix())
	if v, ok := c.fromCache(key); ok {
		return v.([]Event), nil
	}

	params := map[string]any{
		"hostids":     hostIDs,
		"output":      []string{"eventid", "r_eventid", "clock", "value", "severity", "name"},
		"time_from":   timeFrom.Unix(),
		"sortfield":   []string{"clock", "eventid"},
		"sortorder":   "DESC",
		"selectHosts": []string{"hostid", "host", "name", "description"},
	}

	var result []Event
	if err := c.call("event.get", params, &result); err != nil {
		return nil, err
	}

	c.setCache(key, result)
	return result, nil
}

// FetchAllTriggers returns triggers filtered by trigger-level tags (manual mode).
func (c *Client) FetchAllTriggers(tags []Tag) ([]Trigger, error) {
	const key = "triggers"
	if v, ok := c.fromCache(key); ok {
		return v.([]Trigger), nil
	}

	params := map[string]any{
		"tags":              tags,
		"maintenance":       false,
		"expandDescription": true,
		"output":            []string{"triggerid", "description", "priority", "status", "value"},
		"selectHosts":       []string{"host", "name", "description"},
		"selectHostGroups":  []string{"groupid"},
		"selectLastEvent":   []string{"name", "clock", "value"},
	}

	var result []Trigger
	if err := c.call("trigger.get", params, &result); err != nil {
		return nil, err
	}

	c.setCache(key, result)
	return result, nil
}

// FetchEvents returns events filtered by trigger-level tags (manual mode).
func (c *Client) FetchEvents(timeFrom time.Time, tags []Tag) ([]Event, error) {
	key := fmt.Sprintf("events:%d", timeFrom.Unix())
	if v, ok := c.fromCache(key); ok {
		return v.([]Event), nil
	}

	params := map[string]any{
		"tags":        tags,
		"output":      []string{"eventid", "r_eventid", "clock", "value", "severity", "name"},
		"time_from":   timeFrom.Unix(),
		"sortfield":   []string{"clock", "eventid"},
		"sortorder":   "DESC",
		"selectHosts": []string{"hostid", "host", "name", "description"},
	}

	var result []Event
	if err := c.call("event.get", params, &result); err != nil {
		return nil, err
	}

	c.setCache(key, result)
	return result, nil
}

func (c *Client) FetchMaintenance(groupIDs []string) ([]Maintenance, error) {
	key := fmt.Sprintf("maintenance:%v", groupIDs)
	if v, ok := c.fromCache(key); ok {
		return v.([]Maintenance), nil
	}

	params := map[string]any{
		"groupids":          groupIDs,
		"output":            "extend",
		"selectHostGroups":  "extend",
		"selectTimeperiods": "extend",
		"selectTags":        "extend",
	}

	var result []Maintenance
	if err := c.call("maintenance.get", params, &result); err != nil {
		return nil, err
	}

	// Keep only one-time maintenances (timeperiod_type == "0")
	filtered := result[:0]
	for _, m := range result {
		for _, tp := range m.TimePeriods {
			if tp.PeriodType == "0" {
				filtered = append(filtered, m)
				break
			}
		}
	}

	c.setCache(key, filtered)
	return filtered, nil
}
