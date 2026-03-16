package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// UserStats holds parsed Prometheus metrics for a single proxy user.
type UserStats struct {
	Label       string
	Connects    int64
	Current     int64
	BytesTotal  int64
	MsgsTotal   int64
}

// FetchProxyStats fetches and parses per-user metrics from mtprotoproxy Prometheus endpoint.
func FetchProxyStats(metricsURL string) ([]UserStats, error) {
	resp, err := http.Get(metricsURL)
	if err != nil {
		return nil, fmt.Errorf("fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	return parseMetrics(resp.Body)
}

func parseMetrics(r io.Reader) ([]UserStats, error) {
	byUser := make(map[string]*UserStats)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Parse lines like: mtprotoproxy_user_connects{user="u1"} 5
		name, label, value := parseMetricLine(line)
		if label == "" {
			continue
		}

		us, ok := byUser[label]
		if !ok {
			us = &UserStats{Label: label}
			byUser[label] = us
		}

		v, _ := strconv.ParseInt(value, 10, 64)

		switch {
		case strings.HasSuffix(name, "_user_connects_curr"):
			us.Current = v
		case strings.HasSuffix(name, "_user_connects"):
			us.Connects = v
		case strings.HasSuffix(name, "_user_octets") && !strings.HasSuffix(name, "_from") && !strings.HasSuffix(name, "_to"):
			us.BytesTotal = v
		case strings.HasSuffix(name, "_user_msgs") && !strings.HasSuffix(name, "_from") && !strings.HasSuffix(name, "_to"):
			us.MsgsTotal = v
		}
	}

	var result []UserStats
	for _, us := range byUser {
		result = append(result, *us)
	}
	return result, scanner.Err()
}

// parseMetricLine parses "metric_name{user="label"} value"
func parseMetricLine(line string) (name, label, value string) {
	braceStart := strings.Index(line, "{")
	if braceStart < 0 {
		return "", "", ""
	}
	name = line[:braceStart]

	braceEnd := strings.Index(line, "}")
	if braceEnd < 0 {
		return "", "", ""
	}

	// Extract user="..." from inside braces.
	inside := line[braceStart+1 : braceEnd]
	for _, part := range strings.Split(inside, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && kv[0] == "user" {
			label = strings.Trim(kv[1], `"`)
		}
	}

	value = strings.TrimSpace(line[braceEnd+1:])
	return name, label, value
}

// FormatBytes formats bytes into a human-readable string.
func FormatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
