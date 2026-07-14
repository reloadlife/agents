package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/reloadlife/agents/internal/clientpty"
	"github.com/reloadlife/agents/internal/cliview"
	"github.com/reloadlife/agents/internal/ctlconfig"
	"github.com/reloadlife/agents/internal/doctor"
	"github.com/reloadlife/agents/internal/selfupdate"
	"github.com/reloadlife/agents/internal/tui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	args := []string{}
	if len(os.Args) > 1 {
		args = os.Args[1:]
	}
	cfgPath := ""
	urlOverride := ""
	tokenOverride := ""
	sshOverride := ""

	// global flags
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "--config", "-C":
			if len(args) < 2 {
				fatal("missing value for %s", args[0])
			}
			cfgPath = args[1]
			args = args[2:]
		case "--url", "-u":
			if len(args) < 2 {
				fatal("missing value for %s", args[0])
			}
			urlOverride = args[1]
			args = args[2:]
		case "--token", "-t":
			if len(args) < 2 {
				fatal("missing value for %s", args[0])
			}
			tokenOverride = args[1]
			args = args[2:]
		case "--ssh", "--ssh-host":
			if len(args) < 2 {
				fatal("missing value for %s", args[0])
			}
			sshOverride = args[1]
			args = args[2:]
		case "--version", "-version":
			fmt.Printf("agentsctl %s commit=%s date=%s\n", version, commit, date)
			return
		case "-h", "--help":
			usage()
			return
		default:
			goto cmd
		}
	}
cmd:
	// no command → launch TUI (primary UX)
	if len(args) == 0 {
		args = []string{"tui"}
	}

	// local commands — no server token required
	switch args[0] {
	case "config":
		os.Exit(cmdConfig(args[1:], cfgPath))
	case "update":
		os.Exit(cmdUpdate(args[1:]))
	case "version":
		fmt.Printf("agentsctl %s commit=%s date=%s\n", version, commit, date)
		return
	case "help":
		usage()
		return
	}

	cfg, err := ctlconfig.Load(cfgPath)
	if err != nil {
		fatal("%v", err)
	}
	if urlOverride != "" {
		cfg.URL = strings.TrimRight(urlOverride, "/")
	}
	if tokenOverride != "" {
		cfg.Token = tokenOverride
	}
	if sshOverride != "" {
		cfg.SSHHost = sshOverride
	}

	c := &client{base: cfg.URL, token: cfg.Token, hc: &http.Client{Timeout: 0}, cfg: cfg}

	// allow: agentsctl status --json
	cmdArgs := args[1:]
	switch args[0] {
	case "status":
		os.Exit(cmdStatus(c, cmdArgs))
	case "tui", "ui":
		os.Exit(cmdTUI(c, cmdArgs))
	case "agents":
		os.Exit(cmdAgents(c, cmdArgs))
	case "workspaces", "ws":
		os.Exit(cmdWorkspaces(c, cmdArgs))
	case "ssh-keys", "sshkeys", "keys":
		os.Exit(cmdSSHKeys(c, cmdArgs))
	case "gh", "github":
		os.Exit(cmdGH(c, cmdArgs))
	case "doctor":
		os.Exit(cmdDoctor(c))
	case "playwright", "pw":
		os.Exit(cmdPlaywright(c, cmdArgs))
	case "session", "sessions":
		os.Exit(cmdSession(c, cmdArgs))
	case "map", "maps":
		os.Exit(cmdMap(c, cmdArgs))
	case "memory", "mem":
		os.Exit(cmdMemory(c, cmdArgs))
	case "jobs":
		os.Exit(cmdJobs(c, cmdArgs))
	case "run":
		fmt.Fprintln(os.Stderr, "warning: 'run' is print/API mode (may use credits). Prefer: agentsctl tui / session open")
		os.Exit(cmdRun(c, cmdArgs))
	case "logs":
		os.Exit(cmdLogs(c, cmdArgs))
	case "cancel":
		os.Exit(cmdCancel(c, cmdArgs))
	case "confirm":
		os.Exit(cmdConfirm(c, cmdArgs))
	case "get":
		os.Exit(cmdGet(c, cmdArgs))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `agentsctl — CLI for agents (agentsd)

  agentsctl                              # default: open TUI session picker

Primary — full remote PTY (WebSocket, no SSH; NOT print/-p):
  agentsctl tui                          # picker · a agent · w workspace · 1-9 start
  agentsctl doctor                       # health check
  agentsctl agents                       # list CLIs on server
  agentsctl workspaces                   # list allowlisted cwds
  agentsctl workspaces clone URL [-name DIR] [--fork] [--branch B]
  agentsctl ssh-keys list|gen|show|delete   # manage server ~/.ssh identities
  agentsctl gh status|login|switch|logout   # GitHub CLI accounts on the server
  agentsctl playwright status|start|stop|restart|install
  agentsctl session start -a claude|grok|codex|opencode|cursor --open
  agentsctl session open [SESSION_ID]
  agentsctl session list | kill ID | delete ID | resume ID | history ID | prune
  agentsctl map generate -r <cwd>        # write .agents/PROJECT_MAP.md on server
  agentsctl map show|status -r <cwd>
  agentsctl memory index -r <cwd>        # index map+docs into FTS memory
  agentsctl memory search -r <cwd> "q"   # search (for agents / RAG)

Config: agentsctl config init|path|show
Update: agentsctl update [--check] [--force] [--version TAG] [--all]
Other:  agentsctl status | version

Docs: https://github.com/reloadlife/agents
`)
}

func cmdUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	check := fs.Bool("check", false, "only check for a newer release")
	force := fs.Bool("force", false, "reinstall even if already on latest")
	all := fs.Bool("all", false, "also update agentsd if installed next to this binary")
	ver := fs.String("version", "", "install a specific tag (e.g. v0.2.2); default latest")
	_ = fs.Parse(args)

	_, err := selfupdate.Run(selfupdate.Options{
		Current:   version,
		Binary:    "agentsctl",
		All:       *all,
		Version:   *ver,
		CheckOnly: *check,
		Force:     *force,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "update: %v\n", err)
		return 1
	}
	return 0
}

func cmdMemory(c *client, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentsctl memory index|search|stats [-r cwd] [query]")
		return 2
	}
	sub := args[0]
	fs := flag.NewFlagSet("memory "+sub, flag.ExitOnError)
	cwd := fs.String("r", ".", "workspace cwd")
	clear := fs.Bool("clear", false, "clear workspace memory before index")
	code := fs.Bool("code", false, "include shallow code samples when indexing")
	genMap := fs.Bool("map", true, "generate project map before index")
	limit := fs.Int("n", 8, "search result limit")
	mode := fs.String("mode", "auto", "search mode: auto|fts|vector")
	asJSON := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args[1:])

	switch sub {
	case "index":
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/memory/index", map[string]any{
			"cwd":          *cwd,
			"clear":        *clear,
			"include_code": *code,
			"generate_map": *genMap,
		}, &out); err != nil {
			fatal("%v", err)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return 0
		}
		fmt.Printf("indexed cwd=%v docs=%v total=%v\n", out["cwd"], out["indexed"], out["docs_total"])
		return 0
	case "search":
		q := strings.Join(fs.Args(), " ")
		if strings.TrimSpace(q) == "" {
			fatal("usage: agentsctl memory search -r <cwd> <query>")
		}
		var out struct {
			Cwd   string `json:"cwd"`
			Query string `json:"query"`
			Mode  string `json:"mode"`
			Hits  []struct {
				Path    string  `json:"path"`
				Title   string  `json:"title"`
				Source  string  `json:"source"`
				Snippet string  `json:"snippet"`
				Rank    float64 `json:"rank"`
				Mode    string  `json:"mode"`
			} `json:"hits"`
		}
		if err := c.json(http.MethodPost, "/v1/memory/search", map[string]any{
			"cwd":   *cwd,
			"query": q,
			"limit": *limit,
			"mode":  *mode,
		}, &out); err != nil {
			fatal("%v", err)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return 0
		}
		if len(out.Hits) == 0 {
			fmt.Println("(no hits)")
			return 0
		}
		for i, h := range out.Hits {
			m := h.Mode
			if m == "" {
				m = "fts"
			}
			fmt.Printf("%d. %s  [%s/%s]\n   %s\n", i+1, h.Path, h.Source, m, strings.ReplaceAll(h.Snippet, "\n", " "))
		}
		return 0
	case "stats":
		path := "/v1/memory/stats"
		if *cwd != "" {
			path += "?cwd=" + url.QueryEscape(*cwd)
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
		fmt.Printf("cwd=%v docs=%v embedded=%v engine=%v model=%v\n",
			out["cwd"], out["docs"], out["docs_embedded"], out["engine"], out["embed_model"])
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown memory subcommand: %s\n", sub)
		return 2
	}
}

func cmdMap(c *client, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentsctl map generate|show|status [-r cwd]")
		return 2
	}
	sub := args[0]
	fs := flag.NewFlagSet("map "+sub, flag.ExitOnError)
	cwd := fs.String("r", ".", "workspace cwd (relative to server workspace_root)")
	asJSON := fs.Bool("json", false, "raw JSON output")
	_ = fs.Parse(args[1:])

	switch sub {
	case "generate", "gen", "create":
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/maps", map[string]string{"cwd": *cwd}, &out); err != nil {
			fatal("%v", err)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return 0
		}
		fmt.Printf("map written for cwd=%v\n", out["cwd"])
		if p, ok := out["map_path"].(string); ok {
			fmt.Printf("  %s\n", p)
		}
		if st, ok := out["status"].(map[string]any); ok {
			fmt.Printf("  stale=%v %v\n", st["stale"], st["reason"])
		}
		return 0
	case "show", "get", "cat":
		path := "/v1/maps?cwd=" + url.QueryEscape(*cwd)
		if *asJSON {
			path += "&format=json"
		}
		res, err := c.do(http.MethodGet, path, nil)
		if err != nil {
			fatal("%v", err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode >= 300 {
			fatal("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
		}
		if *asJSON {
			fmt.Print(string(b))
			if len(b) == 0 || b[len(b)-1] != '\n' {
				fmt.Println()
			}
			return 0
		}
		var wrap struct {
			Markdown string `json:"markdown"`
		}
		if err := json.Unmarshal(b, &wrap); err == nil && wrap.Markdown != "" {
			fmt.Print(wrap.Markdown)
			if !strings.HasSuffix(wrap.Markdown, "\n") {
				fmt.Println()
			}
			return 0
		}
		fmt.Print(string(b))
		return 0
	case "status", "st":
		var out map[string]any
		if err := c.json(http.MethodGet, "/v1/maps/status?cwd="+url.QueryEscape(*cwd), nil, &out); err != nil {
			fatal("%v", err)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return 0
		}
		st, _ := out["status"].(map[string]any)
		if st == nil {
			fmt.Println(out)
			return 0
		}
		fmt.Printf("cwd=%v exists=%v stale=%v\n", out["cwd"], st["exists"], st["stale"])
		if r, ok := st["reason"].(string); ok && r != "" {
			fmt.Printf("  reason: %s\n", r)
		}
		if g, ok := st["generated_at"].(string); ok && g != "" {
			fmt.Printf("  generated_at: %s\n", g)
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown map subcommand: %s\n", sub)
		return 2
	}
}

func cmdPlaywright(c *client, args []string) int {
	if len(args) == 0 {
		args = []string{"status"}
	}
	// long timeouts for start/install
	old := c.hc
	c.hc = &http.Client{Timeout: 0}
	defer func() { c.hc = old }()

	switch args[0] {
	case "status", "st":
		var st map[string]any
		if err := c.json(http.MethodGet, "/v1/playwright", nil, &st); err != nil {
			fatal("%v", err)
		}
		printPlaywrightStatus(st)
		return 0
	case "start":
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/playwright/start", map[string]any{}, &out); err != nil {
			fatal("%v", err)
		}
		printPlaywrightAction(out)
		return boolExit(out)
	case "stop":
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/playwright/stop", map[string]any{}, &out); err != nil {
			fatal("%v", err)
		}
		printPlaywrightAction(out)
		return boolExit(out)
	case "restart":
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/playwright/restart", map[string]any{}, &out); err != nil {
			fatal("%v", err)
		}
		printPlaywrightAction(out)
		return boolExit(out)
	case "install":
		fmt.Fprintln(os.Stderr, "installing Chromium browsers on server (may take a few minutes)…")
		var out map[string]any
		if err := c.json(http.MethodPost, "/v1/playwright/install", map[string]any{}, &out); err != nil {
			fatal("%v", err)
		}
		if o, _ := out["output"].(string); o != "" {
			fmt.Println(strings.TrimSpace(o))
		}
		printPlaywrightAction(out)
		return boolExit(out)
	default:
		fmt.Fprintln(os.Stderr, "usage: agentsctl playwright status|start|stop|restart|install")
		return 2
	}
}

func printPlaywrightStatus(st map[string]any) {
	fmt.Printf("display     %v  (%v)\n", st["display"], st["display_ok"])
	fmt.Printf("xvfb        %v\n", st["xvfb"])
	fmt.Printf("container   %v  (%v)\n", st["container"], st["container_name"])
	fmt.Printf("server      %v  (%v)\n", st["server"], st["server_ok"])
	fmt.Printf("browsers    path=%v ok=%v\n", st["browsers_path"], st["browsers_ok"])
	if m, _ := st["message"].(string); m != "" {
		fmt.Printf("summary     %s\n", m)
	}
	if cf, _ := st["compose_file"].(string); cf != "" {
		fmt.Printf("compose     %s\n", cf)
	}
}

func printPlaywrightAction(out map[string]any) {
	ok, _ := out["ok"].(bool)
	if err, _ := out["error"].(string); err != "" {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
	}
	if st, okm := out["status"].(map[string]any); okm {
		printPlaywrightStatus(st)
	}
	if ok {
		fmt.Println("ok")
	}
}

func boolExit(out map[string]any) int {
	if ok, _ := out["ok"].(bool); ok {
		return 0
	}
	return 1
}

func cmdDoctor(c *client) int {
	rep := doctor.Run(c.base, c.token)
	for _, ch := range rep.Checks {
		mark := "✓"
		if !ch.OK {
			mark = "✗"
		}
		fmt.Printf("%s %-14s  %s\n", mark, ch.Name, ch.Detail)
	}
	fmt.Println()
	if rep.OK {
		fmt.Println("ok —", rep.Summary)
	} else {
		fmt.Println("fail —", rep.Summary)
		for _, h := range rep.Hints {
			fmt.Println("  ·", h)
		}
		return 1
	}
	return 0
}

func cmdWorkspaces(c *client, args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "clone", "c":
			return cmdWorkspaceClone(c, args[1:])
		case "list", "ls":
			args = args[1:]
		case "help", "-h", "--help":
			fmt.Fprintln(os.Stderr, "usage: agentsctl workspaces [list] | clone URL [--name DIR] [--fork] [--branch B] [--depth N]")
			return 2
		}
	}
	asJSON := false
	for _, a := range args {
		if a == "--json" || a == "-j" {
			asJSON = true
		}
	}
	res, err := c.do(http.MethodGet, "/v1/workspaces", nil)
	if err != nil {
		fatal("%v", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		fatal("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if asJSON {
		var buf bytes.Buffer
		_ = json.Indent(&buf, b, "", "  ")
		fmt.Println(buf.String())
		return 0
	}
	var out struct {
		Root       string `json:"workspace_root"`
		Default    string `json:"default_cwd"`
		Workspaces []struct {
			Path    string `json:"path"`
			Default bool   `json:"default"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("root: %s  default: %s\n\n", out.Root, out.Default)
	for _, w := range out.Workspaces {
		mark := " "
		if w.Default {
			mark = "*"
		}
		fmt.Printf("  %s %s\n", mark, w.Path)
	}
	fmt.Println("\nclone: agentsctl workspaces clone owner/repo [--name DIR] [--fork]")
	fmt.Println("start: agentsctl session start -r <path> -a claude --open")
	return 0
}

func cmdGH(c *client, args []string) int {
	if len(args) == 0 {
		args = []string{"status"}
	}
	switch args[0] {
	case "status", "list", "ls", "accounts":
		return cmdGHStatus(c)
	case "login":
		return cmdGHLogin(c, args[1:])
	case "switch", "use":
		return cmdGHSwitch(c, args[1:])
	case "logout", "rm":
		return cmdGHLogout(c, args[1:])
	case "setup-git":
		return cmdGHSetupGit(c)
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: agentsctl gh status|login|switch|logout|setup-git")
		return 2
	default:
		fmt.Fprintf(os.Stderr, "unknown gh command: %s\n", args[0])
		return 2
	}
}

func cmdGHStatus(c *client) int {
	var out struct {
		OK       bool   `json:"ok"`
		Active   string `json:"active"`
		Binary   string `json:"binary"`
		Error    string `json:"error"`
		Accounts []struct {
			Host        string   `json:"host"`
			User        string   `json:"user"`
			Active      bool     `json:"active"`
			GitProtocol string   `json:"git_protocol"`
			Scopes      []string `json:"scopes"`
		} `json:"accounts"`
	}
	if err := c.json(http.MethodGet, "/v1/gh/accounts", nil, &out); err != nil {
		fatal("%v", err)
	}
	if out.Binary != "" {
		fmt.Printf("gh: %s\n", out.Binary)
	}
	if out.Active != "" {
		fmt.Printf("active: %s\n\n", out.Active)
	} else {
		fmt.Println("active: (none)")
		fmt.Println()
	}
	if len(out.Accounts) == 0 {
		fmt.Println("(no accounts)")
		if out.Error != "" {
			fmt.Println(out.Error)
		}
		fmt.Println("login: agentsctl gh login --token ghp_…")
		return 0
	}
	for _, a := range out.Accounts {
		mark := " "
		if a.Active {
			mark = "*"
		}
		fmt.Printf("  %s %-20s  %s  proto=%s\n", mark, a.User, a.Host, a.GitProtocol)
		if len(a.Scopes) > 0 {
			fmt.Printf("      scopes: %s\n", strings.Join(a.Scopes, ", "))
		}
	}
	return 0
}

func cmdGHLogin(c *client, args []string) int {
	fs := flag.NewFlagSet("gh login", flag.ExitOnError)
	token := fs.String("token", "", "GitHub token (or set GH_TOKEN / pipe via --token-env)")
	tokenEnv := fs.String("token-env", "", "read token from this env var (e.g. GH_TOKEN)")
	host := fs.String("hostname", "github.com", "GitHub host")
	proto := fs.String("git-protocol", "https", "https or ssh")
	insecure := fs.Bool("insecure-storage", true, "store in hosts.yml (typical for headless agentsd)")
	_ = fs.Parse(args)

	tok := strings.TrimSpace(*token)
	if tok == "" && *tokenEnv != "" {
		tok = strings.TrimSpace(os.Getenv(*tokenEnv))
	}
	if tok == "" {
		// also accept GH_TOKEN / GITHUB_TOKEN
		tok = strings.TrimSpace(os.Getenv("GH_TOKEN"))
		if tok == "" {
			tok = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
		}
	}
	if tok == "" {
		// stdin if not a tty
		fi, _ := os.Stdin.Stat()
		if fi != nil && (fi.Mode()&os.ModeCharDevice) == 0 {
			b, _ := io.ReadAll(os.Stdin)
			tok = strings.TrimSpace(string(b))
		}
	}
	if tok == "" {
		fatal("usage: agentsctl gh login --token TOKEN | --token-env GH_TOKEN | GH_TOKEN=… agentsctl gh login")
	}

	body := map[string]any{
		"token":            tok,
		"host":             *host,
		"git_protocol":     *proto,
		"insecure_storage": *insecure,
	}
	var out map[string]any
	if err := c.json(http.MethodPost, "/v1/gh/login", body, &out); err != nil {
		fatal("%v", err)
	}
	if active, _ := out["active"].(string); active != "" {
		fmt.Printf("logged in — active %s\n", active)
	} else {
		fmt.Println("logged in")
	}
	return 0
}

func cmdGHSwitch(c *client, args []string) int {
	fs := flag.NewFlagSet("gh switch", flag.ExitOnError)
	user := fs.String("user", "", "account username")
	host := fs.String("hostname", "github.com", "GitHub host")
	_ = fs.Parse(args)
	u := *user
	if u == "" && fs.NArg() >= 1 {
		u = fs.Arg(0)
	}
	if u == "" {
		fatal("usage: agentsctl gh switch --user USERNAME")
	}
	var out map[string]any
	if err := c.json(http.MethodPost, "/v1/gh/switch", map[string]any{
		"user": u,
		"host": *host,
	}, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("active: %v\n", out["active"])
	return 0
}

func cmdGHLogout(c *client, args []string) int {
	fs := flag.NewFlagSet("gh logout", flag.ExitOnError)
	user := fs.String("user", "", "account username")
	host := fs.String("hostname", "github.com", "GitHub host")
	_ = fs.Parse(args)
	u := *user
	if u == "" && fs.NArg() >= 1 {
		u = fs.Arg(0)
	}
	body := map[string]any{"host": *host}
	if u != "" {
		body["user"] = u
	}
	var out map[string]any
	if err := c.json(http.MethodPost, "/v1/gh/logout", body, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Println("logged out")
	if active, _ := out["active"].(string); active != "" {
		fmt.Printf("active now: %s\n", active)
	}
	return 0
}

func cmdGHSetupGit(c *client) int {
	var out map[string]any
	if err := c.json(http.MethodPost, "/v1/gh/setup-git", map[string]any{}, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Println("git credential helper configured via gh")
	return 0
}

func cmdSSHKeys(c *client, args []string) int {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list", "ls":
		return cmdSSHKeysList(c, args[1:])
	case "gen", "generate", "new", "create":
		return cmdSSHKeysGen(c, args[1:])
	case "show", "get", "pub", "public":
		return cmdSSHKeysShow(c, args[1:])
	case "delete", "rm", "remove":
		return cmdSSHKeysDelete(c, args[1:])
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: agentsctl ssh-keys list|gen NAME|show NAME|delete NAME")
		return 2
	default:
		fmt.Fprintf(os.Stderr, "unknown ssh-keys command: %s\n", args[0])
		return 2
	}
}

func cmdSSHKeysList(c *client, args []string) int {
	var out struct {
		Dir  string `json:"dir"`
		Keys []struct {
			Name        string `json:"name"`
			Type        string `json:"type"`
			Fingerprint string `json:"fingerprint"`
			Comment     string `json:"comment"`
			HasPrivate  bool   `json:"has_private"`
			HasPublic   bool   `json:"has_public"`
		} `json:"keys"`
	}
	if err := c.json(http.MethodGet, "/v1/ssh-keys", nil, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("dir: %s\n\n", out.Dir)
	if len(out.Keys) == 0 {
		fmt.Println("(no keys)")
		fmt.Println("create: agentsctl ssh-keys gen id_agents --comment you@host")
		return 0
	}
	for _, k := range out.Keys {
		priv := "pub-only"
		if k.HasPrivate && k.HasPublic {
			priv = "pair"
		} else if k.HasPrivate {
			priv = "private"
		}
		fmt.Printf("  %-20s  %-8s  %s\n", k.Name, k.Type, priv)
		if k.Fingerprint != "" {
			fmt.Printf("    %s\n", k.Fingerprint)
		}
		if k.Comment != "" {
			fmt.Printf("    %s\n", k.Comment)
		}
	}
	return 0
}

func cmdSSHKeysGen(c *client, args []string) int {
	fs := flag.NewFlagSet("ssh-keys gen", flag.ExitOnError)
	typ := fs.String("type", "ed25519", "ed25519 or rsa")
	comment := fs.String("comment", "", "key comment (default agents@hostname)")
	overwrite := fs.Bool("overwrite", false, "replace existing key")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fatal("usage: agentsctl ssh-keys gen NAME [--type ed25519|rsa] [--comment TEXT] [--overwrite]")
	}
	body := map[string]any{
		"name":      fs.Arg(0),
		"type":      *typ,
		"overwrite": *overwrite,
	}
	if *comment != "" {
		body["comment"] = *comment
	}
	var k map[string]any
	if err := c.json(http.MethodPost, "/v1/ssh-keys", body, &k); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("generated %v (%v)\n", k["name"], k["type"])
	if fp, _ := k["fingerprint"].(string); fp != "" {
		fmt.Println(fp)
	}
	if pub, _ := k["public_key"].(string); pub != "" {
		fmt.Println()
		fmt.Println(pub)
	}
	return 0
}

func cmdSSHKeysShow(c *client, args []string) int {
	if len(args) < 1 {
		fatal("usage: agentsctl ssh-keys show NAME")
	}
	var k map[string]any
	if err := c.json(http.MethodGet, "/v1/ssh-keys/"+args[0], nil, &k); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("name:        %v\n", k["name"])
	fmt.Printf("type:        %v\n", k["type"])
	fmt.Printf("fingerprint: %v\n", k["fingerprint"])
	fmt.Printf("comment:     %v\n", k["comment"])
	if pub, _ := k["public_key"].(string); pub != "" {
		fmt.Println()
		fmt.Println(pub)
	}
	return 0
}

func cmdSSHKeysDelete(c *client, args []string) int {
	if len(args) < 1 {
		fatal("usage: agentsctl ssh-keys delete NAME")
	}
	name := args[0]
	var out map[string]any
	if err := c.json(http.MethodPost, "/v1/ssh-keys/"+name+"/delete", map[string]any{}, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("deleted %s\n", name)
	return 0
}

func cmdWorkspaceClone(c *client, args []string) int {
	fs := flag.NewFlagSet("workspaces clone", flag.ExitOnError)
	name := fs.String("name", "", "directory name under workspace_root")
	branch := fs.String("branch", "", "branch to checkout")
	depth := fs.Int("depth", 0, "shallow clone depth (0 = full)")
	fork := fs.Bool("fork", false, "fork on GitHub first (requires gh auth on server)")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fatal("usage: agentsctl workspaces clone URL|owner/repo [--name DIR] [--fork] [--branch B] [--depth N]")
	}
	body := map[string]any{
		"url":  fs.Arg(0),
		"fork": *fork,
	}
	if *name != "" {
		body["name"] = *name
	}
	if *branch != "" {
		body["branch"] = *branch
	}
	if *depth > 0 {
		body["depth"] = *depth
	}
	var out map[string]any
	if err := c.json(http.MethodPost, "/v1/workspaces/clone", body, &out); err != nil {
		fatal("%v", err)
	}
	cwd, _ := out["cwd"].(string)
	abs, _ := out["abs"].(string)
	fmt.Printf("cloned → %s\n", cwd)
	if abs != "" {
		fmt.Printf("abs: %s\n", abs)
	}
	if f, _ := out["forked"].(bool); f {
		fmt.Println("(forked on GitHub)")
	}
	fmt.Printf("\nnext: agentsctl session start -r %s -a claude --open\n", cwd)
	return 0
}

func cmdAgents(c *client, args []string) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" || a == "-j" {
			asJSON = true
		}
	}
	res, err := c.do(http.MethodGet, "/v1/agents", nil)
	if err != nil {
		fatal("%v", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		fatal("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if asJSON {
		var buf bytes.Buffer
		_ = json.Indent(&buf, b, "", "  ")
		fmt.Println(buf.String())
		return 0
	}
	var out struct {
		Agents []struct {
			Name      string `json:"name"`
			Bin       string `json:"bin"`
			Resolved  string `json:"resolved"`
			Available bool   `json:"available"`
			Note      string `json:"note"`
		} `json:"agents"`
		Available []string `json:"available"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("%-14s  %-10s  %-12s  %s\n", "NAME", "STATUS", "BIN", "NOTE")
	for _, a := range out.Agents {
		if a.Name == "mock" || a.Name == "cursor-agent" {
			continue
		}
		st := "missing"
		if a.Available {
			st = "ok"
		}
		fmt.Printf("%-14s  %-10s  %-12s  %s\n", a.Name, st, a.Bin, a.Note)
	}
	fmt.Printf("\nstart: agentsctl session start -a <%s> --open\n", strings.Join(out.Available, "|"))
	return 0
}

func cmdTUI(c *client, args []string) int {
	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	cwd := fs.String("r", ".", "default cwd for new sessions (. = workspace root)")
	agentName := fs.String("a", "claude", "default agent for new sessions")
	_ = fs.Parse(args)
	err := tui.Run(tui.Config{
		BaseURL:      c.base,
		Token:        c.token,
		DefaultCwd:   *cwd,
		DefaultAgent: *agentName,
	})
	if err != nil {
		fatal("%v", err)
	}
	return 0
}

func cmdConfig(args []string, cfgPath string) int {
	if cfgPath == "" {
		cfgPath = ctlconfig.DefaultPath()
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentsctl config init|path|show")
		return 2
	}
	switch args[0] {
	case "init":
		if err := ctlconfig.WriteExample(cfgPath); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("wrote %s\n", cfgPath)
		fmt.Println("edit token/url/ssh_host, then: chmod 600", cfgPath)
		return 0
	case "path":
		fmt.Println(cfgPath)
		return 0
	case "show":
		cfg, err := ctlconfig.Load(cfgPath)
		if err != nil {
			fatal("%v", err)
		}
		tok := cfg.Token
		if len(tok) > 6 {
			tok = tok[:3] + "…" + tok[len(tok)-2:]
		}
		fmt.Printf("path:       %s\n", cfgPath)
		fmt.Printf("url:        %s\n", cfg.URL)
		fmt.Printf("token:      %s\n", tok)
		fmt.Printf("ssh_host:   %s\n", cfg.SSHHost)
		fmt.Printf("prefer_ssh: %v\n", cfg.PreferSSH)
		fmt.Printf("local_api:  %v\n", cfg.IsLocalAPI())
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown config command: %s\n", args[0])
		return 2
	}
}

type client struct {
	base  string
	token string
	hc    *http.Client
	cfg   *ctlconfig.Config
}

func (c *client) do(method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.hc.Do(req)
}

func (c *client) json(method, path string, body any, out any) error {
	res, err := c.do(method, path, body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(b, out)
}

func cmdStatus(c *client, args []string) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" || a == "-j" {
			asJSON = true
		}
	}
	res, err := c.do(http.MethodGet, "/v1/status", nil)
	if err != nil {
		fatal("%v", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		fatal("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if asJSON {
		var buf bytes.Buffer
		if err := json.Indent(&buf, b, "", "  "); err != nil {
			os.Stdout.Write(b)
			fmt.Println()
			return 0
		}
		fmt.Println(buf.String())
		return 0
	}
	if err := cliview.RenderStatus(os.Stdout, b, c.base); err != nil {
		fatal("%v", err)
	}
	return 0
}

func cmdSession(c *client, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentsctl session start|open|list|attach|kill|delete|resume|history|get|prune ...")
		return 2
	}
	switch args[0] {
	case "start":
		return cmdSessionStart(c, args[1:])
	case "open", "o":
		return cmdSessionOpen(c, args[1:])
	case "list", "ls":
		return cmdSessionList(c, args[1:])
	case "attach", "a":
		return cmdSessionAttachLocal(c, args[1:])
	case "kill", "stop":
		return cmdSessionKill(c, args[1:])
	case "delete", "rm", "remove":
		return cmdSessionDelete(c, args[1:])
	case "resume", "restart":
		return cmdSessionResume(c, args[1:])
	case "history", "log":
		return cmdSessionHistory(c, args[1:])
	case "prune":
		return cmdSessionPrune(c, args[1:])
	case "get":
		return cmdSessionGet(c, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown session command: %s\n", args[0])
		return 2
	}
}

func cmdSessionPrune(c *client, args []string) int {
	fs := flag.NewFlagSet("session prune", flag.ExitOnError)
	maxAge := fs.String("max-age", "", "only remove non-running older than this (e.g. 24h); empty = all stopped")
	_ = fs.Parse(args)
	body := map[string]any{}
	if *maxAge != "" {
		body["max_age"] = *maxAge
	}
	var out map[string]any
	if err := c.json(http.MethodPost, "/v1/sessions/prune", body, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("removed %v stopped session(s)\n", out["removed"])
	return 0
}

func cmdSessionStart(c *client, args []string) int {
	fs := flag.NewFlagSet("session start", flag.ExitOnError)
	repo := fs.String("r", ".", "repo/cwd relative to workspace (. = workspace root)")
	agentName := fs.String("a", "claude", "agent (default claude interactive TTY)")
	name := fs.String("name", "", "optional label")
	doOpen := fs.Bool("open", false, "open full PTY immediately after start")
	doAttach := fs.Bool("attach", false, "same as --open")
	useSSH := fs.Bool("ssh", false, "use SSH attach instead of WebSocket PTY")
	_ = fs.Parse(args)
	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))

	body := map[string]any{
		"agent":  *agentName,
		"cwd":    *repo,
		"mode":   "tty",
		"name":   *name,
		"prompt": prompt,
	}
	var sess map[string]any
	if err := c.json(http.MethodPost, "/v1/sessions", body, &sess); err != nil {
		fatal("%v", err)
	}
	printSessionSummary(sess)
	if *doOpen || *doAttach {
		fmt.Fprintln(os.Stderr, "tip: closing the PTY detaches only — session keeps running until session kill")
		return openSession(c, sess, *useSSH)
	}
	id, _ := sess["id"].(string)
	fmt.Printf("\nnext: agentsctl session open %s\n", id)
	fmt.Println("(detach leaves session running; kill with: agentsctl session kill " + id + ")")
	return 0
}

func printSessionSummary(sess map[string]any) {
	id, _ := sess["id"].(string)
	tmux, _ := sess["tmux"].(string)
	pty, _ := sess["pty_path"].(string)
	fmt.Printf("session %s  agent=%v  cwd=%v  mode=tty\n", id, sess["agent"], sess["cwd"])
	fmt.Printf("tmux:   %s\n", tmux)
	if pty != "" {
		fmt.Printf("pty:    %s  (WebSocket full TTY)\n", pty)
	}
	fmt.Println("(subscription Claude — not API -p)")
}

func cmdSessionOpen(c *client, args []string) int {
	fs := flag.NewFlagSet("session open", flag.ExitOnError)
	useSSH := fs.Bool("ssh", false, "fallback: ssh -t tmux attach")
	_ = fs.Parse(args)

	id := ""
	if fs.NArg() >= 1 {
		id = fs.Arg(0)
	}
	var sess map[string]any
	if id == "" {
		var out struct {
			Sessions []map[string]any `json:"sessions"`
		}
		if err := c.json(http.MethodGet, "/v1/sessions", nil, &out); err != nil {
			fatal("%v", err)
		}
		for _, s := range out.Sessions {
			if s["state"] == "running" {
				sess = s
				break
			}
		}
		if sess == nil {
			fatal("no running sessions — try: agentsctl tui   or   session start -r REPO -a claude --open")
		}
		id, _ = sess["id"].(string)
		fmt.Fprintf(os.Stderr, "opening %s\n", id)
	} else {
		if err := c.json(http.MethodGet, "/v1/sessions/"+id, nil, &sess); err != nil {
			fatal("%v", err)
		}
	}
	return openSession(c, sess, *useSSH)
}

func openSession(c *client, sess map[string]any, forceSSH bool) int {
	id, _ := sess["id"].(string)
	if st, _ := sess["state"].(string); st != "" && st != "running" {
		fatal("session not running (state=%v) — try: agentsctl session resume %s", sess["state"], id)
	}

	if forceSSH {
		tmux, _ := sess["tmux"].(string)
		host := c.cfg.SSHHost
		if host == "" {
			host = "agents"
		}
		fmt.Fprintf(os.Stderr, "→ ssh -t %s -- tmux attach -t %s\n", host, tmux)
		return execCmd([]string{"ssh", "-t", host, "--", "tmux", "attach", "-t", tmux})
	}

	// Default: full remote PTY over WebSocket (no SSH)
	fmt.Fprintf(os.Stderr, "→ PTY %s/v1/sessions/%s/pty\n", c.base, id)
	if err := clientpty.Attach(c.base, c.token, id); err != nil {
		fatal("pty attach: %v\n(fallback: agentsctl session open %s --ssh)", err, id)
	}
	return 0
}

func formatCmd(parts []string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(' ')
		}
		if strings.ContainsAny(p, " \t") {
			b.WriteString("'")
			b.WriteString(p)
			b.WriteString("'")
		} else {
			b.WriteString(p)
		}
	}
	return b.String()
}

func execCmd(parts []string) int {
	if len(parts) < 1 {
		fatal("empty command")
	}
	bin, err := exec.LookPath(parts[0])
	if err != nil {
		fatal("%q not found in PATH: %v", parts[0], err)
	}
	// replace process for proper TTY
	err = syscall.Exec(bin, parts, os.Environ())
	if err != nil {
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fatal("exec failed: %v", err)
		}
	}
	return 0
}

func cmdSessionList(c *client, args []string) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" || a == "-j" {
			asJSON = true
		}
	}
	res, err := c.do(http.MethodGet, "/v1/sessions", nil)
	if err != nil {
		fatal("%v", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		fatal("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if asJSON {
		var buf bytes.Buffer
		_ = json.Indent(&buf, b, "", "  ")
		fmt.Println(buf.String())
		return 0
	}
	if err := cliview.RenderSessionList(os.Stdout, b); err != nil {
		fatal("%v", err)
	}
	return 0
}

func cmdSessionGet(c *client, args []string) int {
	if len(args) < 1 {
		fatal("usage: agentsctl session get SESSION_ID")
	}
	var s map[string]any
	if err := c.json(http.MethodGet, "/v1/sessions/"+args[0], nil, &s); err != nil {
		fatal("%v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s)
	return 0
}

func cmdSessionAttachLocal(c *client, args []string) int {
	if len(args) < 1 {
		fatal("usage: agentsctl session attach SESSION_ID  (local tmux only; prefer: session open)")
	}
	var s map[string]any
	if err := c.json(http.MethodGet, "/v1/sessions/"+args[0], nil, &s); err != nil {
		fatal("%v", err)
	}
	attach, _ := s["attach"].(string)
	if attach == "" {
		fatal("no attach command")
	}
	fmt.Fprintln(os.Stderr, "note: local tmux attach — for remote use: agentsctl session open")
	return execCmd(strings.Fields(attach))
}

func cmdSessionResume(c *client, args []string) int {
	fs := flag.NewFlagSet("session resume", flag.ExitOnError)
	doOpen := fs.Bool("open", false, "open full PTY immediately after resume")
	useSSH := fs.Bool("ssh", false, "use SSH attach instead of WebSocket PTY")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fatal("usage: agentsctl session resume SESSION_ID [--open]")
	}
	id := fs.Arg(0)
	var s map[string]any
	if err := c.json(http.MethodPost, "/v1/sessions/"+id+"/resume", map[string]any{}, &s); err != nil {
		fatal("%v", err)
	}
	printSessionSummary(s)
	fmt.Println("(resume restarts the agent process if tmux was gone; terminal scrollback is restored from snapshot when available)")
	if *doOpen {
		return openSession(c, s, *useSSH)
	}
	fmt.Printf("\nnext: agentsctl session open %s\n", id)
	return 0
}

func cmdSessionHistory(c *client, args []string) int {
	if len(args) < 1 {
		fatal("usage: agentsctl session history SESSION_ID")
	}
	id := args[0]
	req, err := http.NewRequest(http.MethodGet, c.base+"/v1/sessions/"+id+"/history", nil)
	if err != nil {
		fatal("%v", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		fatal("%v", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		fatal("history: HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	if src := res.Header.Get("X-Agents-History-Source"); src != "" {
		fmt.Fprintf(os.Stderr, "source: %s (%d bytes)\n", src, len(b))
	}
	_, _ = os.Stdout.Write(b)
	if len(b) > 0 && b[len(b)-1] != '\n' {
		fmt.Println()
	}
	return 0
}

func cmdSessionDelete(c *client, args []string) int {
	if len(args) < 1 {
		fatal("usage: agentsctl session delete SESSION_ID")
	}
	id := args[0]
	var out map[string]any
	if err := c.json(http.MethodPost, "/v1/sessions/"+id+"/delete", map[string]any{}, &out); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("deleted %s\n", id)
	return 0
}

func cmdSessionKill(c *client, args []string) int {
	if len(args) < 1 {
		fatal("usage: agentsctl session kill SESSION_ID")
	}
	var s map[string]any
	if err := c.json(http.MethodPost, "/v1/sessions/"+args[0]+"/kill", map[string]any{}, &s); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("%s state=%v\n", s["id"], s["state"])
	return 0
}

func cmdJobs(c *client, args []string) int {
	asJSON := false
	for _, a := range args {
		if a == "--json" || a == "-j" {
			asJSON = true
		}
	}
	var out struct {
		Jobs []map[string]any `json:"jobs"`
	}
	if err := c.json(http.MethodGet, "/v1/jobs", nil, &out); err != nil {
		fatal("%v", err)
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	if len(out.Jobs) == 0 {
		fmt.Println("no print jobs")
		return 0
	}
	fmt.Printf("%-28s  %-12s  %-8s  %s\n", "ID", "STATE", "AGENT", "CWD")
	for _, j := range out.Jobs {
		fmt.Printf("%-28s  %-12v  %-8v  %v\n", j["id"], j["state"], j["agent"], j["cwd"])
	}
	return 0
}

func cmdGet(c *client, args []string) int {
	if len(args) < 1 {
		fatal("usage: agentsctl get JOB_ID")
	}
	var j map[string]any
	if err := c.json(http.MethodGet, "/v1/jobs/"+args[0], nil, &j); err != nil {
		fatal("%v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(j)
	return 0
}

func cmdRun(c *client, args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	repo := fs.String("r", ".", "repo/cwd relative to workspace")
	agentName := fs.String("a", "mock", "agent name")
	follow := fs.Bool("follow", false, "follow SSE until done")
	timeout := fs.String("timeout", "", "timeout duration e.g. 15m")
	var caps multiFlag
	fs.Var(&caps, "cap", "capability (repeatable)")
	_ = fs.Parse(args)
	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" {
		fatal("prompt required")
	}
	body := map[string]any{
		"prompt": prompt,
		"agent":  *agentName,
		"cwd":    *repo,
		"caps":   []string(caps),
	}
	if *timeout != "" {
		body["timeout"] = *timeout
	}
	var resp map[string]any
	if err := c.json(http.MethodPost, "/v1/jobs", body, &resp); err != nil {
		fatal("%v", err)
	}
	id, _ := resp["id"].(string)
	fmt.Printf("job %s state=%v\n", id, resp["state"])
	if tok, ok := resp["confirm_token"].(string); ok && tok != "" {
		fmt.Printf("confirm_token: %s\n", tok)
	}
	if !*follow {
		return 0
	}
	return followSSE(c, id)
}

func cmdLogs(c *client, args []string) int {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("f", false, "follow")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fatal("usage: agentsctl logs JOB_ID [-f]")
	}
	id := fs.Arg(0)
	if *follow {
		return followSSE(c, id)
	}
	res, err := c.do(http.MethodGet, "/v1/jobs/"+id+"/log", nil)
	if err != nil {
		fatal("%v", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		fatal("HTTP %d: %s", res.StatusCode, b)
	}
	_, _ = io.Copy(os.Stdout, res.Body)
	return 0
}

func cmdCancel(c *client, args []string) int {
	if len(args) < 1 {
		fatal("usage: agentsctl cancel JOB_ID")
	}
	var j map[string]any
	if err := c.json(http.MethodPost, "/v1/jobs/"+args[0]+"/cancel", map[string]any{}, &j); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("%s state=%v\n", j["id"], j["state"])
	return 0
}

func cmdConfirm(c *client, args []string) int {
	fs := flag.NewFlagSet("confirm", flag.ExitOnError)
	tok := fs.String("token", "", "confirm token")
	_ = fs.Parse(args)
	if fs.NArg() < 1 || *tok == "" {
		fatal("usage: agentsctl confirm JOB_ID --token TOKEN")
	}
	var j map[string]any
	if err := c.json(http.MethodPost, "/v1/jobs/"+fs.Arg(0)+"/confirm", map[string]string{"token": *tok}, &j); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("%s state=%v\n", j["id"], j["state"])
	return 0
}

func followSSE(c *client, id string) int {
	hc := &http.Client{Timeout: 0}
	req, err := http.NewRequest(http.MethodGet, c.base+"/v1/jobs/"+id+"/events", nil)
	if err != nil {
		fatal("%v", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := hc.Do(req)
	if err != nil {
		fatal("%v", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		fatal("HTTP %d: %s", res.StatusCode, b)
	}
	sc := bufio.NewScanner(res.Body)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	var event string
	exitCode := 0
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			switch event {
			case "log":
				var m struct {
					Line string `json:"line"`
				}
				_ = json.Unmarshal([]byte(data), &m)
				fmt.Println(m.Line)
			case "state":
				var m struct {
					State string `json:"state"`
				}
				_ = json.Unmarshal([]byte(data), &m)
				fmt.Fprintf(os.Stderr, "[state] %s\n", m.State)
			case "result":
				var m struct {
					ExitCode *int   `json:"exit_code"`
					State    string `json:"state"`
					Error    string `json:"error"`
				}
				_ = json.Unmarshal([]byte(data), &m)
				fmt.Fprintf(os.Stderr, "[result] state=%s exit=%v err=%s\n", m.State, m.ExitCode, m.Error)
				if m.ExitCode != nil {
					exitCode = *m.ExitCode
				} else if m.State != "succeeded" {
					exitCode = 1
				}
				return exitCode
			}
		}
	}
	if err := sc.Err(); err != nil {
		fatal("stream: %v", err)
	}
	return exitCode
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

var _ = time.Second
