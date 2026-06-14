package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"zabbix-statuspage/internal/config"
	"zabbix-statuspage/internal/zabbix"
	"github.com/labstack/echo/v5"
)

const uptimeSlots = 90

type HostData struct {
	ZabbixHost    string
	DisplayHost   string
	Label         string
	Description   string
	HasProblem    bool
	StatusLabel   string
	SeverityLabel string
	UptimeBars    []bool
	UptimePct     float64
	Triggers      []zabbix.Trigger
}

type HistoryItem struct {
	ClockUnix   string
	ClockISO    string
	Name        string
	Resolved    bool
	HostLabel   string
	ResolvedISO string
}

type MaintenanceItem struct {
	Name        string
	Description string
	SinceUnix   string
	SinceISO    string
	TillUnix    string
	TillISO     string
}

type SegmentData struct {
	Name        string
	Description string
	Services    []HostData
}

type PageData struct {
	Compact             bool
	Micro               bool
	Segments            []SegmentData
	Hosts               []HostData
	History             []HistoryItem
	Upcoming            []MaintenanceItem
	ExternalStatuspages []config.ExternalLink
}

type Summary struct {
	Hosts   int
	OK      int
	Problem int
}

type TemplateData struct {
	Data           PageData
	CurrentDateISO string
	Summary        Summary
	Debug          bool
	Version        string
}

type StatusHandler struct {
	zabbixClient *zabbix.Client
	cfg          *config.Config
	debug        bool
	version      string
}

func New(client *zabbix.Client, cfg *config.Config, debug bool, version string) *StatusHandler {
	return &StatusHandler{
		zabbixClient: client,
		cfg:          cfg,
		debug:        debug,
		version:      version,
	}
}

func unixToISO(clockStr string) string {
	unix, err := strconv.ParseInt(clockStr, 10, 64)
	if err != nil {
		return ""
	}
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

func computeUptimeBars(hostEvents []zabbix.Event, resolvedClocks map[string]string, now time.Time) ([]bool, float64) {
	slotDur := 24 * time.Hour / uptimeSlots
	windowStart := now.Add(-24 * time.Hour)

	bars := make([]bool, uptimeSlots)
	for i := range bars {
		bars[i] = true
	}

	for _, e := range hostEvents {
		if e.Value != "1" {
			continue
		}
		pStartUnix, err := strconv.ParseInt(e.Clock, 10, 64)
		if err != nil {
			continue
		}
		pStart := time.Unix(pStartUnix, 0)

		var pEnd time.Time
		if e.REventID != "0" {
			if rc, ok := resolvedClocks[e.REventID]; ok {
				if endUnix, err := strconv.ParseInt(rc, 10, 64); err == nil {
					pEnd = time.Unix(endUnix, 0)
				}
			}
		}
		if pEnd.IsZero() {
			pEnd = now
		}
		if pEnd.Before(windowStart) {
			continue
		}

		for i := 0; i < uptimeSlots; i++ {
			slotStart := windowStart.Add(time.Duration(i) * slotDur)
			slotEnd := slotStart.Add(slotDur)
			if pStart.Before(slotEnd) && pEnd.After(slotStart) {
				bars[i] = false
			}
		}
	}

	okCount := 0
	for _, ok := range bars {
		if ok {
			okCount++
		}
	}
	return bars, float64(okCount) / float64(uptimeSlots) * 100
}

func statusPriority(label string) int {
	switch label {
	case "Outage":
		return 0
	case "Degraded":
		return 1
	default:
		return 2
	}
}

func sortServices(services []HostData) {
	sort.SliceStable(services, func(i, j int) bool {
		return statusPriority(services[i].StatusLabel) < statusPriority(services[j].StatusLabel)
	})
}

func deriveStatusLabel(triggers []zabbix.Trigger) string {
	label := "Operational"
	for _, t := range triggers {
		if t.Value != "1" {
			continue
		}
		if t.Priority == "4" || t.Priority == "5" {
			return "Outage"
		}
		label = "Degraded"
	}
	return label
}

func priorityLabel(p int) string {
	switch p {
	case 0:
		return "Not classified"
	case 1:
		return "Information"
	case 2:
		return "Warning"
	case 3:
		return "Average"
	case 4:
		return "High"
	case 5:
		return "Disaster"
	default:
		return ""
	}
}

func highestActiveSeverity(triggers []zabbix.Trigger) string {
	highest := -1
	for _, t := range triggers {
		if t.Value != "1" {
			continue
		}
		p, err := strconv.Atoi(t.Priority)
		if err != nil {
			continue
		}
		if p > highest {
			highest = p
		}
	}
	if highest < 0 {
		return ""
	}
	return priorityLabel(highest)
}

func buildHostData(
	host string, label string, description string,
	triggers []zabbix.Trigger,
	eventsByHost map[string][]zabbix.Event,
	resolvedClocks map[string]string,
	now time.Time,
) HostData {
	hasProblem := false
	for _, t := range triggers {
		if t.Value == "1" {
			hasProblem = true
			break
		}
	}
	bars, pct := computeUptimeBars(eventsByHost[host], resolvedClocks, now)
	return HostData{
		ZabbixHost:    host,
		Label:         label,
		Description:   description,
		HasProblem:    hasProblem,
		StatusLabel:   deriveStatusLabel(triggers),
		SeverityLabel: highestActiveSeverity(triggers),
		UptimeBars:    bars,
		UptimePct:     pct,
		Triggers:      triggers,
	}
}

func (h *StatusHandler) Handle(c *echo.Context) error {
	compact := c.QueryParam("compact") == "1"
	micro := c.QueryParam("micro") == "1"

	tags := make([]zabbix.Tag, len(h.cfg.TriggerTags))
	for i, t := range h.cfg.TriggerTags {
		tags[i] = zabbix.Tag{Tag: t.Tag, Value: t.Value}
	}

	now := time.Now()
	backHistory := now.Add(-3 * 24 * time.Hour)

	autoDiscover := len(h.cfg.Segments) == 0

	var (
		allTriggers []zabbix.Trigger
		events      []zabbix.Event
		err         error
	)

	if autoDiscover {
		// ── Auto-discover mode: find hosts by host-level tag ──────────────
		discoveredHosts, err := h.zabbixClient.FetchHostsByTags(tags)
		if err != nil {
			return renderError(c, err)
		}

		hostIDs := make([]string, len(discoveredHosts))
		for i, dh := range discoveredHosts {
			hostIDs[i] = dh.HostID
		}

		allTriggers, err = h.zabbixClient.FetchTriggersByHostIDs(hostIDs)
		if err != nil {
			return renderError(c, err)
		}

		events, err = h.zabbixClient.FetchEventsByHostIDs(hostIDs, backHistory)
		if err != nil {
			return renderError(c, err)
		}

		// Build segments from discovered hosts
		triggersByHost := make(map[string][]zabbix.Trigger)
		for _, t := range allTriggers {
			for _, host := range t.Hosts {
				triggersByHost[host.Host] = append(triggersByHost[host.Host], t)
			}
		}

		resolvedClocks := resolvedClocksMap(events)
		eventsByHost := eventsByHostMap(events)

		groupIDSet := make(map[string]struct{})
		for _, t := range allTriggers {
			for _, hg := range t.HostGroups {
				groupIDSet[hg.GroupID] = struct{}{}
			}
		}
		groupIDs := make([]string, 0, len(groupIDSet))
		for id := range groupIDSet {
			groupIDs = append(groupIDs, id)
		}

		maintenances, err := h.zabbixClient.FetchMaintenance(groupIDs)
		if err != nil {
			return renderError(c, err)
		}

		sd := SegmentData{Name: "Services"}
		hostLabels := make(map[string]string)
		summary := Summary{}
		var flatHosts []HostData

		for _, dh := range discoveredHosts {
			label := dh.DisplayName()
			triggers := triggersByHost[dh.Host]
			hd := buildHostData(dh.Host, label, dh.Description, triggers, eventsByHost, resolvedClocks, now)
			hostLabels[dh.Host] = label
			if hd.HasProblem {
				summary.Problem++
			} else {
				summary.OK++
			}
			summary.Hosts++
			sd.Services = append(sd.Services, hd)
			flatHosts = append(flatHosts, hd)
		}

		sortServices(sd.Services)

		historyItems := buildHistory(events, resolvedClocks, hostLabels)
		upcomingItems := buildMaintenance(maintenances)

		return c.Render(http.StatusOK, "index.html", TemplateData{
			Data: PageData{
				Compact:             compact,
				Micro:               micro,
				Segments:            []SegmentData{sd},
				Hosts:               flatHosts,
				History:             historyItems,
				Upcoming:            upcomingItems,
				ExternalStatuspages: h.cfg.ExternalStatuspages,
			},
			CurrentDateISO: now.UTC().Format(time.RFC3339),
			Summary:        summary,
			Debug:          h.debug,
			Version:        h.version,
		})
	}

	// ── Manual mode: segments defined in config, filter by trigger tags ──
	allTriggers, err = h.zabbixClient.FetchAllTriggers(tags)
	if err != nil {
		return renderError(c, err)
	}

	triggersByHost := make(map[string][]zabbix.Trigger)
	for _, t := range allTriggers {
		for _, host := range t.Hosts {
			triggersByHost[host.Host] = append(triggersByHost[host.Host], t)
		}
	}

	groupIDSet := make(map[string]struct{})
	for _, t := range allTriggers {
		for _, hg := range t.HostGroups {
			groupIDSet[hg.GroupID] = struct{}{}
		}
	}
	groupIDs := make([]string, 0, len(groupIDSet))
	for id := range groupIDSet {
		groupIDs = append(groupIDs, id)
	}

	events, err = h.zabbixClient.FetchEvents(backHistory, tags)
	if err != nil {
		return renderError(c, err)
	}

	maintenances, err := h.zabbixClient.FetchMaintenance(groupIDs)
	if err != nil {
		return renderError(c, err)
	}

	resolvedClocks := resolvedClocksMap(events)
	eventsByHost := eventsByHostMap(events)

	hostLabels := make(map[string]string)
	var segments []SegmentData
	var flatHosts []HostData
	summary := Summary{}

	for _, seg := range h.cfg.Segments {
		sd := SegmentData{Name: seg.Name, Description: seg.Description}
		for _, svc := range seg.Services {
			triggers := triggersByHost[svc.ZabbixHost]
			label := svc.ZabbixHost
			if svc.DisplayHost != "" {
				label = svc.DisplayHost
			}
			desc := svc.Description
			if desc == "" && len(triggers) > 0 && len(triggers[0].Hosts) > 0 {
				desc = triggers[0].Hosts[0].Description
			}
			hd := buildHostData(svc.ZabbixHost, label, desc, triggers, eventsByHost, resolvedClocks, now)
			hd.DisplayHost = svc.DisplayHost
			hostLabels[svc.ZabbixHost] = label
			if hd.HasProblem {
				summary.Problem++
			} else {
				summary.OK++
			}
			summary.Hosts++
			sd.Services = append(sd.Services, hd)
			flatHosts = append(flatHosts, hd)
		}
		sortServices(sd.Services)
		segments = append(segments, sd)
	}

	historyItems := buildHistory(events, resolvedClocks, hostLabels)
	upcomingItems := buildMaintenance(maintenances)

	return c.Render(http.StatusOK, "index.html", TemplateData{
		Data: PageData{
			Compact:             compact,
			Micro:               micro,
			Segments:            segments,
			Hosts:               flatHosts,
			History:             historyItems,
			Upcoming:            upcomingItems,
			ExternalStatuspages: h.cfg.ExternalStatuspages,
		},
		CurrentDateISO: now.UTC().Format(time.RFC3339),
		Summary:        summary,
		Debug:          h.debug,
		Version:        h.version,
	})
}

func resolvedClocksMap(events []zabbix.Event) map[string]string {
	m := make(map[string]string)
	for _, e := range events {
		if e.Value == "0" {
			m[e.EventID] = e.Clock
		}
	}
	return m
}

func eventsByHostMap(events []zabbix.Event) map[string][]zabbix.Event {
	m := make(map[string][]zabbix.Event)
	for _, e := range events {
		for _, h := range e.Hosts {
			m[h.Host] = append(m[h.Host], e)
		}
	}
	return m
}

func buildHistory(events []zabbix.Event, resolvedClocks map[string]string, hostLabels map[string]string) []HistoryItem {
	var items []HistoryItem
	for _, e := range events {
		if e.REventID == "0" {
			continue
		}
		hostLabel := ""
		if len(e.Hosts) > 0 {
			if lbl, ok := hostLabels[e.Hosts[0].Host]; ok {
				hostLabel = lbl
			} else {
				hostLabel = e.Hosts[0].Host
			}
		}
		resolvedISO := ""
		if rc, ok := resolvedClocks[e.REventID]; ok {
			resolvedISO = unixToISO(rc)
		}
		items = append(items, HistoryItem{
			ClockUnix:   e.Clock,
			ClockISO:    unixToISO(e.Clock),
			Name:        e.Name,
			Resolved:    true,
			HostLabel:   hostLabel,
			ResolvedISO: resolvedISO,
		})
	}
	return items
}

func buildMaintenance(maintenances []zabbix.Maintenance) []MaintenanceItem {
	var items []MaintenanceItem
	for _, m := range maintenances {
		items = append(items, MaintenanceItem{
			Name:        m.Name,
			Description: m.Description,
			SinceUnix:   m.ActiveSince,
			SinceISO:    unixToISO(m.ActiveSince),
			TillUnix:    m.ActiveTill,
			TillISO:     unixToISO(m.ActiveTill),
		})
	}
	return items
}

func renderError(c *echo.Context, err error) error {
	return c.Render(http.StatusInternalServerError, "error.html", map[string]string{
		"Error": fmt.Sprintf("%v", err),
	})
}
