package httpapi

import (
	"golang.org/x/sys/unix"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Host proc is a read-only, explicitly configured mount; never enable privileged mode.
func (a *API) runtimeSummary(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]any{"scope": "host", "timezone": "Asia/Shanghai", "sampledAt": time.Now(), "procAvailable": false}
	proc := a.Config.HostProcDir
	if proc == "" {
		proc = "/proc"
	}
	if raw, err := os.ReadFile(filepath.Join(proc, "loadavg")); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) >= 3 {
			load := []float64{}
			for _, v := range fields[:3] {
				n, _ := strconv.ParseFloat(v, 64)
				load = append(load, n)
			}
			metrics["loadAverage"] = load
			metrics["procAvailable"] = true
		}
	}
	if raw, err := os.ReadFile(filepath.Join(proc, "meminfo")); err == nil {
		values := map[string]uint64{}
		for _, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			if len(f) > 1 {
				n, _ := strconv.ParseUint(f[1], 10, 64)
				values[strings.TrimSuffix(f[0], ":")] = n * 1024
			}
		}
		if total := values["MemTotal"]; total > 0 {
			available := values["MemAvailable"]
			metrics["memoryTotal"] = total
			metrics["memoryAvailable"] = available
			metrics["memoryUsed"] = total - min(total, available)
		}
	}
	if raw, err := os.ReadFile(filepath.Join(proc, "uptime")); err == nil {
		f := strings.Fields(string(raw))
		if len(f) > 0 {
			n, _ := strconv.ParseFloat(f[0], 64)
			metrics["uptimeSeconds"] = n
		}
	}
	var disk unix.Statfs_t
	if err := unix.Statfs(a.Config.STRMOutputDir, &disk); err == nil {
		metrics["diskTotal"] = uint64(disk.Blocks) * uint64(disk.Bsize)
		metrics["diskAvailable"] = uint64(disk.Bavail) * uint64(disk.Bsize)
		metrics["diskScope"] = "STRM 数据目录所在文件系统"
	}
	counts := map[string]int64{}
	errors := []string{}
	if a.Deps.DB != nil {
		for name, sql := range map[string]string{
			"sites":         "SELECT count(*) FROM integrations WHERE kind='ptsite' AND enabled",
			"subscriptions": "SELECT count(*) FROM subscriptions WHERE enabled",
			"downloads":     "SELECT count(*) FROM download_tasks WHERE download_state NOT IN ('failed','cancelled')",
			"transfers":     "SELECT count(*) FROM transfer_tasks WHERE state NOT IN ('failed','completed','cancelled')",
			"media":         "SELECT count(*) FROM media_assets",
			"todaySearches": "SELECT count(*) FROM search_runs WHERE created_at >= date_trunc('day',now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai'",
			"failedJobs":    "SELECT count(*) FROM jobs WHERE state='failed'",
		} {
			var n int64
			if err := a.Deps.DB.QueryRow(r.Context(), sql).Scan(&n); err == nil {
				counts[name] = n
			} else {
				errors = append(errors, name)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, map[string]any{"server": metrics, "daily": counts, "unavailableCounts": errors})
}
