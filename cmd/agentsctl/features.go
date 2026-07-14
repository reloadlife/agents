package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func cmdRecordings(c *client, args []string) int {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub := args[0]
	fs := flag.NewFlagSet("recordings "+sub, flag.ExitOnError)
	sid := fs.String("session", "", "filter by session id")
	q := fs.String("q", "", "search pane text")
	limit := fs.Int("limit", 50, "max results")
	asJSON := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args[1:])

	switch sub {
	case "list", "ls":
		path := fmt.Sprintf("/v1/recordings?limit=%d", *limit)
		if *sid != "" {
			path += "&session_id=" + url.QueryEscape(*sid)
		}
		if *q != "" {
			path += "&q=" + url.QueryEscape(*q)
		}
		var out map[string]any
		if err := c.json(http.MethodGet, path, nil, &out); err != nil {
			fatal("%v", err)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return 0
		}
		fmt.Printf("enabled=%v\n", out["enabled"])
		recs, _ := out["recordings"].([]any)
		for _, r := range recs {
			m, _ := r.(map[string]any)
			fmt.Printf("%v  sess=%v  agent=%v  cwd=%v  bytes=%v  %v\n",
				m["id"], m["session_id"], m["agent"], m["cwd"], m["bytes"], m["created_at"])
		}
		return 0
	case "show", "get":
		if len(fs.Args()) < 1 {
			fatal("usage: agentsctl recordings show <id>")
		}
		id := fs.Args()[0]
		var out map[string]any
		if err := c.json(http.MethodGet, "/v1/recordings/"+url.PathEscape(id), nil, &out); err != nil {
			fatal("%v", err)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return 0
		}
		if t, ok := out["text"].(string); ok {
			fmt.Print(t)
		}
		return 0
	case "snap", "capture":
		if len(fs.Args()) < 1 {
			fatal("usage: agentsctl recordings snap <session_id>")
		}
		id := fs.Args()[0]
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/sessions/"+url.PathEscape(id)+"/record", map[string]any{}, &out); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("ok=%v\n", out["ok"])
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: agentsctl recordings list|show|snap")
		return 2
	}
}

func cmdTemplates(c *client, args []string) int {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub := args[0]
	fs := flag.NewFlagSet("templates "+sub, flag.ExitOnError)
	name := fs.String("name", "", "template name")
	agent := fs.String("a", "claude", "agent")
	cwd := fs.String("r", ".", "workspace cwd")
	prompt := fs.String("prompt", "", "seed prompt")
	account := fs.String("account", "", "account profile id")
	ensure := fs.Bool("ensure", true, "ensure context on start")
	id := fs.String("id", "", "template id (update)")
	asJSON := fs.Bool("json", false, "JSON")
	_ = fs.Parse(args[1:])

	switch sub {
	case "list", "ls":
		var out map[string]any
		if err := c.json(http.MethodGet, "/v1/templates", nil, &out); err != nil {
			fatal("%v", err)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return 0
		}
		list, _ := out["templates"].([]any)
		for _, raw := range list {
			t, _ := raw.(map[string]any)
			fmt.Printf("%v  %v  agent=%v  cwd=%v\n", t["id"], t["name"], t["agent"], t["cwd"])
		}
		return 0
	case "save", "add", "upsert":
		if *name == "" {
			fatal("usage: agentsctl templates save --name NAME -a agent -r cwd [--prompt …]")
		}
		body := map[string]any{
			"name":           *name,
			"agent":          *agent,
			"cwd":            *cwd,
			"prompt":         *prompt,
			"account":        *account,
			"ensure_context": *ensure,
		}
		if *id != "" {
			body["id"] = *id
		}
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/templates", body, &out); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("saved %v (%v)\n", out["name"], out["id"])
		return 0
	case "delete", "rm":
		tid := *id
		if tid == "" && len(fs.Args()) > 0 {
			tid = fs.Args()[0]
		}
		if tid == "" {
			fatal("usage: agentsctl templates delete <id>")
		}
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/templates/"+url.PathEscape(tid)+"/delete", map[string]any{}, &out); err != nil {
			fatal("%v", err)
		}
		fmt.Println("deleted")
		return 0
	case "start", "run":
		tid := *id
		if tid == "" && len(fs.Args()) > 0 {
			tid = fs.Args()[0]
		}
		if tid == "" {
			fatal("usage: agentsctl templates start <id>")
		}
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/templates/"+url.PathEscape(tid)+"/start", map[string]any{}, &out); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("started session %v  agent=%v  cwd=%v\n", out["id"], out["agent"], out["cwd"])
		fmt.Printf("open: agentsctl session open %v\n", out["id"])
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: agentsctl templates list|save|delete|start")
		return 2
	}
}

func cmdAudit(c *client, args []string) int {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	limit := fs.Int("limit", 50, "entries")
	asJSON := fs.Bool("json", false, "JSON")
	_ = fs.Parse(args)
	var out map[string]any
	if err := c.json(http.MethodGet, fmt.Sprintf("/v1/audit?limit=%d", *limit), nil, &out); err != nil {
		fatal("%v", err)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	entries, _ := out["entries"].([]any)
	for _, raw := range entries {
		e, _ := raw.(map[string]any)
		fmt.Printf("%v  %v  actor=%v  target=%v\n", e["at"], e["action"], e["actor"], e["target"])
	}
	return 0
}

func cmdBackup(c *client, args []string) int {
	if len(args) == 0 {
		args = []string{"create"}
	}
	switch args[0] {
	case "create", "new":
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/backup", map[string]any{}, &out); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("backup: %v\n", out["path"])
		return 0
	case "restore":
		if len(args) < 2 {
			fatal("usage: agentsctl backup restore <path-under-jobs_dir>")
		}
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/backup/restore", map[string]any{"path": args[1]}, &out); err != nil {
			fatal("%v", err)
		}
		fmt.Println("restored — restart agentsd if sessions look stale")
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: agentsctl backup create|restore")
		return 2
	}
}

func cmdDashboard(c *client, args []string) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" || a == "-json" {
			asJSON = true
		}
	}
	var out map[string]any
	if err := c.json(http.MethodGet, "/v1/dashboard", nil, &out); err != nil {
		fatal("%v", err)
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	list, _ := out["workspaces"].([]any)
	for _, raw := range list {
		w, _ := raw.(map[string]any)
		fmt.Printf("%-20v  map=%v stale=%v ctx=%v mem=%v live=%v\n",
			w["path"], w["map_exists"], w["map_stale"], w["has_context"], w["memory_docs"], w["live_sessions"])
	}
	return 0
}

func cmdHistorySearch(c *client, args []string) int {
	if len(args) == 0 {
		fatal("usage: agentsctl history search <query>")
	}
	q := strings.Join(args, " ")
	if args[0] == "search" {
		q = strings.Join(args[1:], " ")
	}
	var out map[string]any
	if err := c.json(http.MethodGet, "/v1/history/search?q="+url.QueryEscape(q), nil, &out); err != nil {
		fatal("%v", err)
	}
	hits, _ := out["hits"].([]any)
	for _, raw := range hits {
		h, _ := raw.(map[string]any)
		fmt.Printf("%v  %v/%v  [%v]\n  %v\n", h["session_id"], h["agent"], h["cwd"], h["source"], h["snippet"])
	}
	if len(hits) == 0 {
		fmt.Println("no hits")
	}
	return 0
}

func cmdSkills(c *client, args []string) int {
	fs := flag.NewFlagSet("skills", flag.ExitOnError)
	cwd := fs.String("r", ".", "workspace")
	_ = fs.Parse(args)
	sub := "install"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
		_ = fs.Parse(args[1:])
	}
	if sub != "install" {
		fmt.Fprintln(os.Stderr, "usage: agentsctl skills install -r <cwd>")
		return 2
	}
	var out map[string]any
	if err := c.json(http.MethodPost, "/v1/skills/install", map[string]any{"cwd": *cwd}, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("installed skill stub at %v (%v)\n", out["path"], out["cwd"])
	return 0
}

func cmdNotify(c *client, args []string) int {
	if len(args) == 0 || args[0] == "test" {
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/notify/test", map[string]any{}, &out); err != nil {
			fatal("%v", err)
		}
		fmt.Println("test event sent")
		return 0
	}
	fmt.Fprintln(os.Stderr, "usage: agentsctl notify test")
	return 2
}
