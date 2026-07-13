package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Report is a client-side health check against a running agentsd.
type Report struct {
	OK       bool     `json:"ok"`
	URL      string   `json:"url"`
	Checks   []Check  `json:"checks"`
	Summary  string   `json:"summary"`
	Hints    []string `json:"hints,omitempty"`
}

type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail,omitempty"`
}

// Run probes the API with the given base URL and token.
func Run(baseURL, token string) Report {
	baseURL = strings.TrimRight(baseURL, "/")
	r := Report{URL: baseURL, OK: true}

	add := func(name string, ok bool, detail string) {
		r.Checks = append(r.Checks, Check{Name: name, OK: ok, Detail: detail})
		if !ok {
			r.OK = false
		}
	}

	// healthz
	{
		client := &http.Client{Timeout: 5 * time.Second}
		res, err := client.Get(baseURL + "/healthz")
		if err != nil {
			add("reachability", false, err.Error())
			r.Summary = "cannot reach agentsd"
			r.Hints = append(r.Hints, "Is agentsd running? Check url in agentsctl config.")
			return r
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			add("reachability", false, fmt.Sprintf("HTTP %d", res.StatusCode))
		} else {
			add("reachability", true, strings.TrimSpace(string(b)))
		}
	}

	// auth + status
	{
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/status", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		client := &http.Client{Timeout: 8 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			add("auth/status", false, err.Error())
		} else {
			b, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			if res.StatusCode == 401 {
				add("auth/status", false, "unauthorized — check token")
				r.Hints = append(r.Hints, "agentsctl config show  # token must match AGENTSD_TOKEN")
			} else if res.StatusCode >= 300 {
				add("auth/status", false, fmt.Sprintf("HTTP %d: %s", res.StatusCode, truncate(string(b), 80)))
			} else {
				var st map[string]any
				_ = json.Unmarshal(b, &st)
				host, _ := st["host"].(string)
				add("auth/status", true, "host="+host)
				if agents, ok := st["agents"].([]any); ok {
					var okN, badN int
					var names []string
					for _, a := range agents {
						m, _ := a.(map[string]any)
						name, _ := m["name"].(string)
						if name == "mock" || name == "cursor-agent" {
							continue
						}
						if av, _ := m["available"].(bool); av {
							okN++
							names = append(names, name)
						} else {
							badN++
						}
					}
					add("agents", okN > 0, fmt.Sprintf("%d available: %s", okN, strings.Join(names, ", ")))
					if badN > 0 {
						r.Hints = append(r.Hints, "some agents missing on server — install CLIs or fix [agents] bin paths")
					}
				}
			}
		}
	}

	// agents endpoint
	{
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/agents", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		client := &http.Client{Timeout: 5 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			add("agents-api", false, err.Error())
		} else {
			_ = res.Body.Close()
			add("agents-api", res.StatusCode == 200, fmt.Sprintf("HTTP %d", res.StatusCode))
		}
	}

	// workspaces
	{
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/workspaces", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		client := &http.Client{Timeout: 5 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			add("workspaces", false, err.Error())
		} else {
			b, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			if res.StatusCode != 200 {
				add("workspaces", false, fmt.Sprintf("HTTP %d", res.StatusCode))
			} else {
				var out struct {
					Workspaces []struct {
						Path string `json:"path"`
					} `json:"workspaces"`
				}
				_ = json.Unmarshal(b, &out)
				add("workspaces", len(out.Workspaces) > 0, fmt.Sprintf("%d path(s)", len(out.Workspaces)))
			}
		}
	}

	// local tmux (only matters if using --ssh fallback on this machine)
	if _, err := exec.LookPath("tmux"); err != nil {
		add("local-tmux", true, "not required for WebSocket PTY")
	} else {
		add("local-tmux", true, "present (ssh fallback ok)")
	}

	if r.OK {
		r.Summary = "all checks passed"
	} else {
		r.Summary = "some checks failed"
	}
	return r
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
