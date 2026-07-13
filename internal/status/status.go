package status

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/reloadlife/agents/internal/config"
)

type Snapshot struct {
	Host      string         `json:"host"`
	Time      time.Time      `json:"time"`
	GoOS      string         `json:"goos"`
	GoArch    string         `json:"goarch"`
	NumCPU    int            `json:"num_cpu"`
	Goroutines int           `json:"goroutines"`
	Mem       MemStat        `json:"mem"`
	Disk      *DiskStat      `json:"disk,omitempty"`
	Load      []float64      `json:"load,omitempty"`
	Docker    string         `json:"docker,omitempty"`
	OpenDray  string         `json:"opendray,omitempty"`
	GKE       *GKEStat       `json:"gke,omitempty"`
	Jobs      JobStat        `json:"jobs"`
	UptimeSec float64        `json:"uptime_sec,omitempty"`
	Display   string         `json:"display,omitempty"`
	DisplayOK string         `json:"display_ok,omitempty"` // active|down|unset
}

type MemStat struct {
	AllocMB     uint64 `json:"alloc_mb"`
	SysMB       uint64 `json:"sys_mb"`
	// host-level if available
	HostTotalMB     uint64 `json:"host_total_mb,omitempty"`
	HostAvailableMB uint64 `json:"host_available_mb,omitempty"`
}

type DiskStat struct {
	Path string  `json:"path"`
	UsedPct float64 `json:"used_pct,omitempty"`
	// free-form from df
	Raw string `json:"raw,omitempty"`
}

type GKEStat struct {
	NodesReady int    `json:"nodes_ready"`
	NodesTotal int    `json:"nodes_total"`
	Error      string `json:"error,omitempty"`
}

type JobStat struct {
	Running int `json:"running"`
	Queued  int `json:"queued"`
}

var startedAt = time.Now()

func Collect(ctx context.Context, cfg *config.Config, running, queued int) Snapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	s := Snapshot{
		Host:       hostname(),
		Time:       time.Now().UTC(),
		GoOS:       runtime.GOOS,
		GoArch:     runtime.GOARCH,
		NumCPU:     runtime.NumCPU(),
		Goroutines: runtime.NumGoroutine(),
		Mem: MemStat{
			AllocMB: ms.Alloc / 1024 / 1024,
			SysMB:   ms.Sys / 1024 / 1024,
		},
		Jobs:      JobStat{Running: running, Queued: queued},
		UptimeSec: time.Since(startedAt).Seconds(),
	}
	s.Load = readLoad()
	s.Disk = readDisk(cfg.WorkspaceRoot)
	s.Docker = probeDocker(ctx)
	if total, avail, ok := readHostMem(); ok {
		s.Mem.HostTotalMB = total
		s.Mem.HostAvailableMB = avail
	}
	s.Display = cfg.Sessions.Display
	if s.Display == "" {
		s.Display = os.Getenv("DISPLAY")
	}
	s.DisplayOK = probeDisplay(s.Display)
	if cfg.Status.OpenDrayURL != "" {
		s.OpenDray = probeHTTP(ctx, cfg.Status.OpenDrayURL)
	}
	if cfg.Status.GKEContext != "" {
		s.GKE = probeGKE(ctx, cfg.Status.GKEContext)
	}
	return s
}

func probeDisplay(display string) string {
	if display == "" {
		return "unset"
	}
	// xdpyinfo is the reliable check when available
	if p, err := exec.LookPath("xdpyinfo"); err == nil {
		cmd := exec.Command(p, "-display", display)
		if err := cmd.Run(); err == nil {
			return "active"
		}
		return "down"
	}
	// fallback: Xvfb lock file for :N → /tmp/.X99-lock
	if strings.HasPrefix(display, ":") {
		num := strings.TrimPrefix(display, ":")
		if _, err := os.Stat("/tmp/.X" + num + "-lock"); err == nil {
			return "active"
		}
		return "down"
	}
	return "unknown"
}

func readHostMem() (totalMB, availMB uint64, ok bool) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	var total, avail uint64
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// values in kB
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = v / 1024
		case "MemAvailable:":
			avail = v / 1024
		}
	}
	if total == 0 {
		return 0, 0, false
	}
	return total, avail, true
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func readLoad() []float64 {
	// Linux: /proc/loadavg — macOS: sysctl via uptime parse (best effort)
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	parts := strings.Fields(string(b))
	if len(parts) < 3 {
		return nil
	}
	var out []float64
	for i := 0; i < 3; i++ {
		f, err := strconv.ParseFloat(parts[i], 64)
		if err != nil {
			return nil
		}
		out = append(out, f)
	}
	return out
}

func readDisk(path string) *DiskStat {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "df", "-h", path)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return &DiskStat{Path: path, Raw: strings.TrimSpace(string(out))}
}

func probeDocker(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		return "down"
	}
	return "active"
}

func probeHTTP(ctx context.Context, url string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "error"
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "down"
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 500 {
		return "active"
	}
	return "down"
}

func probeGKE(ctx context.Context, contextName string) *GKEStat {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	home, _ := os.UserHomeDir()
	if h := os.Getenv("HOME"); h != "" {
		home = h
	}
	kubectl := lookPath("kubectl",
		"/usr/local/bin/kubectl",
		filepath.Join(home, "google-cloud-sdk", "bin", "kubectl"),
		"/usr/lib/google-cloud-sdk/bin/kubectl",
	)
	if kubectl == "" {
		return &GKEStat{Error: "kubectl not found"}
	}

	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, kubectl, args...)
		cmd.Env = kubectlEnv()
		return cmd.CombinedOutput()
	}

	var out []byte
	var err error
	if contextName != "" {
		out, err = run("--context", contextName, "get", "nodes", "-o", "json")
	}
	if err != nil || contextName == "" {
		out2, err2 := run("get", "nodes", "-o", "json")
		if err2 == nil {
			out, err = out2, nil
		} else if err == nil {
			err = err2
			out = out2
		} else {
			// prefer second error message if both fail
			err = fmt.Errorf("%v; fallback: %v", trimErr(err, out), trimErr(err2, out2))
		}
	}
	if err != nil {
		return &GKEStat{Error: trimErr(err, out)}
	}

	var parsed struct {
		Items []struct {
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return &GKEStat{Error: "invalid kubectl json: " + err.Error()}
	}
	ready := 0
	for _, n := range parsed.Items {
		for _, c := range n.Status.Conditions {
			if c.Type == "Ready" && c.Status == "True" {
				ready++
			}
		}
	}
	return &GKEStat{NodesReady: ready, NodesTotal: len(parsed.Items)}
}

func lookPath(name string, extras ...string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, p := range extras {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func kubectlEnv() []string {
	// Minimal env so gke-gcloud-auth-plugin + kubeconfig work under systemd
	home := os.Getenv("HOME")
	if home == "" {
		if u, err := os.UserHomeDir(); err == nil {
			home = u
		}
	}
	gcloudBin := filepath.Join(home, "google-cloud-sdk", "bin")
	path := os.Getenv("PATH")
	if path == "" {
		path = gcloudBin + ":/usr/local/bin:/usr/bin:/bin"
	} else if !strings.Contains(path, "google-cloud-sdk") {
		path = gcloudBin + ":" + path
	}
	user := envOr("USER", "")
	if user == "" {
		user = "user"
	}
	env := []string{
		"HOME=" + home,
		"PATH=" + path,
		"USER=" + user,
		"USE_GKE_GCLOUD_AUTH_PLUGIN=True",
		"CLOUDSDK_CORE_DISABLE_PROMPTS=1",
		"KUBECONFIG=" + envOr("KUBECONFIG", filepath.Join(home, ".kube", "config")),
	}
	for _, k := range []string{"GOOGLE_APPLICATION_CREDENTIALS", "CLOUDSDK_CONFIG", "LANG", "LC_ALL"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func trimErr(err error, out []byte) string {
	msg := strings.TrimSpace(string(out))
	if msg == "" && err != nil {
		return err.Error()
	}
	// last line is usually the real error
	lines := strings.Split(msg, "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return err.Error()
	}
	if len(last) > 120 {
		last = last[:117] + "…"
	}
	return last
}
