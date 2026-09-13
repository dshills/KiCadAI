package airequirementeval

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func directoryBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in evidence tree: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("nonregular evidence artifact")
		}
		total += info.Size()
		return nil
	})
	return total, err
}

type processRow struct {
	pid, parent int
	rss         int64
}

func processTreeRSS(ctx context.Context) (int64, error) {
	data, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,rss=").Output()
	if err != nil {
		return 0, err
	}
	rows := []processRow{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 3 {
			return 0, fmt.Errorf("unparseable RSS sample")
		}
		pid, e1 := strconv.Atoi(fields[0])
		parent, e2 := strconv.Atoi(fields[1])
		rss, e3 := strconv.ParseInt(fields[2], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || rss < 0 {
			return 0, fmt.Errorf("invalid RSS sample")
		}
		rows = append(rows, processRow{pid, parent, rss * 1024})
	}
	members := map[int]bool{os.Getpid(): true}
	for changed := true; changed; {
		changed = false
		for _, row := range rows {
			if members[row.parent] && !members[row.pid] {
				members[row.pid] = true
				changed = true
			}
		}
	}
	var total int64
	for _, row := range rows {
		if members[row.pid] {
			total += row.rss
		}
	}
	if total <= 0 {
		return 0, fmt.Errorf("empty RSS sample")
	}
	return total, nil
}

type resourceMonitor struct {
	mu         sync.Mutex
	PeakRSS    int64  `json:"peak_sampled_process_tree_rss_bytes"`
	PeakDisk   int64  `json:"peak_sampled_evidence_bytes"`
	Samples    int    `json:"samples"`
	Errors     int    `json:"sampling_errors"`
	StopReason string `json:"stop_reason"`
}

func (m *resourceMonitor) sample(ctx context.Context, root string) error {
	sampleCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rss, rssErr := processTreeRSS(sampleCtx)
	disk, diskErr := directoryBytes(root)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Samples++
	m.PeakRSS = max(m.PeakRSS, rss)
	m.PeakDisk = max(m.PeakDisk, disk)
	if rssErr != nil || diskErr != nil {
		m.Errors++
		m.StopReason = "resource_sample_failed"
		return fmt.Errorf("resource sampling failed")
	}
	if rss > 16<<30 {
		m.StopReason = "rss_limit"
		return fmt.Errorf("sampled RSS exceeded 16 GiB")
	}
	if disk > EvidenceBytes {
		m.StopReason = "evidence_limit"
		return fmt.Errorf("evidence exceeded 1 GiB")
	}
	return nil
}

func (m *resourceMonitor) watch(ctx context.Context, root string, cancel context.CancelFunc, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.sample(ctx, root); err != nil {
				if ctx.Err() == nil {
					cancel()
				}
				return
			}
		}
	}
}

func (m *resourceMonitor) snapshot() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]any{"peak_sampled_process_tree_rss_bytes": m.PeakRSS, "peak_sampled_evidence_bytes": m.PeakDisk, "samples": m.Samples, "sampling_errors": m.Errors, "stop_reason": m.StopReason, "sample_interval_ms": 500, "instantaneous_peak_is_not_proven": true}
}
