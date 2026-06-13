package zabbix

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func mockServer(t *testing.T, method string, result any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		raw, _ := json.Marshal(result)
		resp := map[string]any{"jsonrpc": "2.0", "id": 1, "result": json.RawMessage(raw)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestFetchAllTriggers(t *testing.T) {
	want := []Trigger{{TriggerID: "1", Value: "0"}}
	srv := mockServer(t, "trigger.get", want)
	defer srv.Close()

	c := New(srv.URL, "token", 30)
	got, err := c.FetchAllTriggers([]Tag{{Tag: "output", Value: "statuspage"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TriggerID != "1" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestFetchAllTriggersCache(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := json.Marshal([]Trigger{})
		resp := map[string]any{"jsonrpc": "2.0", "id": 1, "result": json.RawMessage(raw)}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "token", 30)
	c.FetchAllTriggers(nil)
	c.FetchAllTriggers(nil)

	if calls != 1 {
		t.Errorf("expected 1 HTTP call (cache hit), got %d", calls)
	}
}

func TestFetchAllTriggersCacheRace(t *testing.T) {
	srv := mockServer(t, "trigger.get", []Trigger{})
	defer srv.Close()

	c := New(srv.URL, "token", 30)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.FetchAllTriggers(nil)
		}()
	}
	wg.Wait()
}

func TestFetchEvents(t *testing.T) {
	want := []Event{{EventID: "42", REventID: "43", Clock: "1700000000"}}
	srv := mockServer(t, "event.get", want)
	defer srv.Close()

	c := New(srv.URL, "token", 30)
	got, err := c.FetchEvents(time.Now().Add(-72*time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EventID != "42" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestFetchMaintenanceFilterOneTime(t *testing.T) {
	all := []Maintenance{
		{Name: "OneTime", TimePeriods: []TimePeriod{{PeriodType: "0"}}},
		{Name: "Weekly", TimePeriods: []TimePeriod{{PeriodType: "1"}}},
	}
	srv := mockServer(t, "maintenance.get", all)
	defer srv.Close()

	c := New(srv.URL, "token", 30)
	got, err := c.FetchMaintenance([]string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "OneTime" {
		t.Errorf("expected 1 one-time maintenance, got: %+v", got)
	}
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"jsonrpc": "2.0", "id": 1,
			"error": map[string]string{"message": "Not authorised", "data": ""},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "bad-token", 30)
	_, err := c.FetchAllTriggers(nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
