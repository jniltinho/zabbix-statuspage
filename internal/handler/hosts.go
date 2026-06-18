package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"zabbix-statuspage/internal/zabbix"

	"github.com/labstack/echo/v5"
)

func zabbixBaseURL(apiURL string) string {
	return strings.TrimSuffix(strings.TrimSuffix(apiURL, "/api_jsonrpc.php"), "/")
}

type resolvedProblemInfo struct {
	StartISO    string
	Duration    string
}

// lastResolvedProblemByHost returns per-host info about the most recent resolved problem.
func lastResolvedProblemByHost(events []zabbix.Event, resolvedClocks map[string]string) map[string]resolvedProblemInfo {
	type raw struct{ startClock, resolvedClock string }
	best := make(map[string]raw)
	for _, e := range events {
		if e.Value != "1" || e.REventID == "0" || e.REventID == "" {
			continue
		}
		rc, ok := resolvedClocks[e.REventID]
		if !ok {
			continue
		}
		for _, h := range e.Hosts {
			if cur, exists := best[h.Host]; !exists || e.Clock > cur.startClock {
				best[h.Host] = raw{e.Clock, rc}
			}
		}
	}
	result := make(map[string]resolvedProblemInfo, len(best))
	for host, r := range best {
		info := resolvedProblemInfo{StartISO: unixToISO(r.startClock)}
		s, err1 := strconv.ParseInt(r.startClock, 10, 64)
		res, err2 := strconv.ParseInt(r.resolvedClock, 10, 64)
		if err1 == nil && err2 == nil {
			info.Duration = formatDuration(time.Duration(res-s) * time.Second)
		}
		result[host] = info
	}
	return result
}

func (h *StatusHandler) HandleHostStatus(c *echo.Context) error {
	tags := make([]zabbix.Tag, len(h.cfg.TriggerTags))
	for i, t := range h.cfg.TriggerTags {
		tags[i] = zabbix.Tag{Tag: t.Tag, Value: t.Value}
	}
	now := time.Now()
	backHistory := now.Add(-3 * 24 * time.Hour)
	baseURL := zabbixBaseURL(h.cfg.Zabbix.APIURL)

	var flatHosts []HostData
	summary := Summary{}

	if len(h.cfg.Segments) == 0 {
		discoveredHosts, err := h.zabbixClient.FetchHostsByTags(tags)
		if err != nil {
			return renderError(c, err)
		}
		hostIDs := make([]string, len(discoveredHosts))
		for i, dh := range discoveredHosts {
			hostIDs[i] = dh.HostID
		}
		allTriggers, err := h.zabbixClient.FetchTriggersByHostIDs(hostIDs)
		if err != nil {
			return renderError(c, err)
		}
		events, err := h.zabbixClient.FetchEventsByHostIDs(hostIDs, backHistory)
		if err != nil {
			return renderError(c, err)
		}
		resolvedClocks := resolvedClocksMap(events)
		resolvedProblems := lastResolvedProblemByHost(events, resolvedClocks)

		triggersByHost := make(map[string][]zabbix.Trigger)
		for _, t := range allTriggers {
			for _, host := range t.Hosts {
				triggersByHost[host.Host] = append(triggersByHost[host.Host], t)
			}
		}
		for _, dh := range discoveredHosts {
			hd := buildHostData(dh.Host, dh.DisplayName(), dh.Description, triggersByHost[dh.Host], nil, nil, now)
			hd.HostID = dh.HostID
			if !hd.HasProblem {
				if info, ok := resolvedProblems[dh.Host]; ok {
					hd.LastProblemStartISO = info.StartISO
					hd.DowntimeDuration = info.Duration
				}
			}
			if hd.HasProblem {
				summary.Problem++
			} else {
				summary.OK++
			}
			summary.Hosts++
			flatHosts = append(flatHosts, hd)
		}
	} else {
		allTriggers, err := h.zabbixClient.FetchAllTriggers(tags)
		if err != nil {
			return renderError(c, err)
		}
		events, err := h.zabbixClient.FetchEvents(backHistory, tags)
		if err != nil {
			return renderError(c, err)
		}
		resolvedClocks := resolvedClocksMap(events)
		resolvedProblems := lastResolvedProblemByHost(events, resolvedClocks)

		triggersByHost := make(map[string][]zabbix.Trigger)
		for _, t := range allTriggers {
			for _, host := range t.Hosts {
				triggersByHost[host.Host] = append(triggersByHost[host.Host], t)
			}
		}
		for _, seg := range h.cfg.Segments {
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
				hd := buildHostData(svc.ZabbixHost, label, desc, triggers, nil, nil, now)
				hd.DisplayHost = svc.DisplayHost
				if !hd.HasProblem {
					if info, ok := resolvedProblems[svc.ZabbixHost]; ok {
						hd.LastProblemStartISO = info.StartISO
						hd.DowntimeDuration = info.Duration
					}
				}
				if hd.HasProblem {
					summary.Problem++
				} else {
					summary.OK++
				}
				summary.Hosts++
				flatHosts = append(flatHosts, hd)
			}
		}
	}

	sortServices(flatHosts)

	return c.Render(http.StatusOK, "hostlist.html", TemplateData{
		Data: PageData{
			Hosts: flatHosts,
		},
		CurrentDateISO: now.UTC().Format(time.RFC3339),
		Summary:        summary,
		Debug:          h.debug,
		Version:        h.version,
		ZabbixBaseURL:  baseURL,
	})
}
