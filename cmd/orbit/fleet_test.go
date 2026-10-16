package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/rig/v3/models"
	sched "github.com/mrsirg97-rgb/rig/v3/store/scheduler"
)

func fleetFetch(running []string, fail bool) sched.Fetch {
	return func(url string) (json.RawMessage, error) {
		if fail {
			return nil, fmt.Errorf("swap down")
		}
		switch {
		case strings.HasSuffix(url, "/v1/models"):
			status := "unloaded"
			for _, m := range running {
				if m == "local" {
					status = "loaded"
				}
			}
			return json.RawMessage(`{"data":[{"id":"local","status":{"value":"` + status + `"}}]}`), nil
		case strings.HasSuffix(url, "/running"):
			return json.RawMessage(`{"running":[]}`), nil
		}
		return nil, fmt.Errorf("unexpected url %s", url)
	}
}

func fleetLocalTable(t *testing.T) models.Table {
	t.Helper()
	tbl, err := models.New(models.Model{ID: "local", Window: 8192, MaxTokens: 1024, Reserve: 64, KeepRecent: 128, Role: models.RoleInteractive})
	if err != nil {
		t.Fatal(err)
	}
	return tbl
}

func fleetRemoteTable(t *testing.T) models.Table {
	t.Helper()
	tbl, err := models.New(models.Model{ID: "brain", Window: 8192, MaxTokens: 1024, Reserve: 64, KeepRecent: 128, Role: models.RoleWorker, Remote: true, BaseURL: "https://endpoint.example/v1"})
	if err != nil {
		t.Fatal(err)
	}
	return tbl
}

func TestFleetWiringNeedsTheWorkerDialOn(t *testing.T) {
	on, why := fleetWiring(fleetFetch([]string{"local"}, false), "http://127.0.0.1:8090", "local", fleetLocalTable(t), false)
	if on {
		t.Error("the dial off is no fleet")
	}
	if !strings.Contains(why, `"workers": false`) {
		t.Errorf("the refusal must name the dial: %q", why)
	}
}

func TestFleetWiringReadsTheSwap(t *testing.T) {
	on, _ := fleetWiring(fleetFetch([]string{"local"}, false), "http://127.0.0.1:8090", "local", fleetLocalTable(t), true)
	if !on {
		t.Error("a readable swap with a resident model is a fleet")
	}
	on, _ = fleetWiring(fleetFetch(nil, false), "http://127.0.0.1:8090", "local", fleetLocalTable(t), true)
	if !on {
		t.Error("a readable swap with nothing resident is a fleet too")
	}
}

func TestFleetWiringRemoteRowSkipsTheProbe(t *testing.T) {
	on, _ := fleetWiring(fleetFetch(nil, true), "http://127.0.0.1:8090", "brain", fleetRemoteTable(t), true)
	if !on {
		t.Error("a remote model row is a fleet without the live probe")
	}
}

func TestFleetWiringUnreadableSwapIsNoFleet(t *testing.T) {
	on, why := fleetWiring(fleetFetch(nil, true), "http://127.0.0.1:8090", "local", fleetLocalTable(t), true)
	if on {
		t.Error("an unreadable swap is no fleet")
	}
	if !strings.Contains(why, "unreadable") {
		t.Errorf("the refusal must name the swap: %q", why)
	}
}
