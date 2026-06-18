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
	ZabbixHost     string
	DisplayHost    string
	HostID         string
	ZabbixBaseURL  string
	Label          string
	Description    string
	HasProblem     bool
	StatusLabel    string
	SeverityLabel  string
	UptimeBars     []bool
	UptimePct      float64
	Triggers       []zabbix.Trigger
	ActiveTriggers      []zabbix.Trigger
	LastEventISO        string
	LastEventName       string
	LastProblemStartISO string
	DowntimeDuration    string
}

type HistoryItem struct {
	ClockUnix   string
	ClockISO    string
	Name        string
	Resolved    bool
	HostLabel   string
	HostID      string
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
	CurrentProblems     []HistoryItem
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
	ZabbixBaseURL  string
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

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Minute)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
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
		pi, pj := statusPriority(services[i].StatusLabel), statusPriority(services[j].StatusLabel)
		if pi != pj {
			return pi < pj
		}
		return services[i].LastEventISO > services[j].LastEventISO
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
	var activeTrigs []zabbix.Trigger
	lastClock := ""
	for _, t := range triggers {
		if t.Value == "1" {
			hasProblem = true
			activeTrigs = append(activeTrigs, t)
			if t.LastEvent.Clock != "" && (lastClock == "" || t.LastEvent.Clock > lastClock) {
				lastClock = t.LastEvent.Clock
			}
		}
	}
	lastEventName := ""
	if lastClock == "" {
		for _, t := range triggers {
			if t.LastEvent.Clock != "" && (lastClock == "" || t.LastEvent.Clock > lastClock) {
				lastClock = t.LastEvent.Clock
				lastEventName = t.LastEvent.Name
				if lastEventName == "" {
					lastEventName = t.Description
				}
			}
		}
	}
	downtimeDuration := ""
	if hasProblem && lastClock != "" {
		if unix, err := strconv.ParseInt(lastClock, 10, 64); err == nil {
			downtimeDuration = formatDuration(now.Sub(time.Unix(unix, 0)))
		}
	}
	bars, pct := computeUptimeBars(eventsByHost[host], resolvedClocks, now)
	return HostData{
		ZabbixHost:      host,
		Label:           label,
		Description:     description,
		HasProblem:      hasProblem,
		StatusLabel:     deriveStatusLabel(triggers),
		SeverityLabel:   highestActiveSeverity(triggers),
		UptimeBars:      bars,
		UptimePct:       pct,
		Triggers:        triggers,
		ActiveTriggers:  activeTrigs,
		LastEventISO:    unixToISO(lastClock),
		LastEventName:   lastEventName,
		DowntimeDuration: downtimeDuration,
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
	backHistory := now.Add(-7 * 24 * time.Hour)

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
			hd.HostID = dh.HostID
			hd.ZabbixBaseURL = zabbixBaseURL(h.cfg.Zabbix.APIURL)
			hostLabels[dh.Host] = label
			if hd.HasProblem {
				summary.Problem++
			} else {
				summary.OK++
			}
			summary.Hosts++
			sd.Services = append(sd.Services, hd)
		}

		sortServices(sd.Services)
		flatHosts = sd.Services

		currentProblems := buildCurrentProblemsFromTriggers(allTriggers, hostLabels)
		historyItems := buildHistory(events, resolvedClocks, hostLabels)
		upcomingItems := buildMaintenance(maintenances)

		return c.Render(http.StatusOK, "index.html", TemplateData{
			Data: PageData{
				Compact:             compact,
				Micro:               micro,
				Segments:            []SegmentData{sd},
				Hosts:               flatHosts,
				CurrentProblems:     currentProblems,
				History:             historyItems,
				Upcoming:            upcomingItems,
				ExternalStatuspages: h.cfg.ExternalStatuspages,
			},
			CurrentDateISO: now.UTC().Format(time.RFC3339),
			Summary:        summary,
			Debug:          h.debug,
			Version:        h.version,
			ZabbixBaseURL:  zabbixBaseURL(h.cfg.Zabbix.APIURL),
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
			if len(triggers) > 0 && len(triggers[0].Hosts) > 0 {
				hd.HostID = triggers[0].Hosts[0].HostID
			}
			hd.ZabbixBaseURL = zabbixBaseURL(h.cfg.Zabbix.APIURL)
			hostLabels[svc.ZabbixHost] = label
			if hd.HasProblem {
				summary.Problem++
			} else {
				summary.OK++
			}
			summary.Hosts++
			sd.Services = append(sd.Services, hd)
		}
		sortServices(sd.Services)
		flatHosts = append(flatHosts, sd.Services...)
		segments = append(segments, sd)
	}

	currentProblems := buildCurrentProblemsFromTriggers(allTriggers, hostLabels)
	historyItems := buildHistory(events, resolvedClocks, hostLabels)
	upcomingItems := buildMaintenance(maintenances)

	return c.Render(http.StatusOK, "index.html", TemplateData{
		Data: PageData{
			Compact:             compact,
			Micro:               micro,
			Segments:            segments,
			Hosts:               flatHosts,
			CurrentProblems:     currentProblems,
			History:             historyItems,
			Upcoming:            upcomingItems,
			ExternalStatuspages: h.cfg.ExternalStatuspages,
		},
		CurrentDateISO: now.UTC().Format(time.RFC3339),
		Summary:        summary,
		Debug:          h.debug,
		Version:        h.version,
		ZabbixBaseURL:  zabbixBaseURL(h.cfg.Zabbix.APIURL),
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

func buildCurrentProblems(events []zabbix.Event, hostLabels map[string]string) []HistoryItem {
	var items []HistoryItem
	for _, e := range events {
		if e.Value != "1" || (e.REventID != "0" && e.REventID != "") {
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
		items = append(items, HistoryItem{
			ClockUnix: e.Clock,
			ClockISO:  unixToISO(e.Clock),
			Name:      e.Name,
			HostLabel: hostLabel,
			Resolved:  false,
		})
	}
	return items
}

func buildCurrentProblemsFromTriggers(triggers []zabbix.Trigger, hostLabels map[string]string) []HistoryItem {
	var items []HistoryItem
	for _, t := range triggers {
		if t.Value != "1" || t.LastEvent.Clock == "" {
			continue
		}
		hostLabel := ""
		if len(t.Hosts) > 0 {
			if lbl, ok := hostLabels[t.Hosts[0].Host]; ok {
				hostLabel = lbl
			} else {
				hostLabel = t.Hosts[0].Host
			}
		}
		name := t.LastEvent.Name
		if name == "" {
			name = t.Description
		}
		items = append(items, HistoryItem{
			ClockUnix: t.LastEvent.Clock,
			ClockISO:  unixToISO(t.LastEvent.Clock),
			Name:      name,
			HostLabel: hostLabel,
			Resolved:  false,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ClockUnix > items[j].ClockUnix
	})
	return items
}

func buildHistory(events []zabbix.Event, resolvedClocks map[string]string, hostLabels map[string]string) []HistoryItem {
	var items []HistoryItem
	for _, e := range events {
		if e.Value != "1" || e.REventID == "0" || e.REventID == "" {
			continue
		}
		hostLabel := ""
		hostID := ""
		if len(e.Hosts) > 0 {
			hostID = e.Hosts[0].HostID
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
			HostID:      hostID,
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
