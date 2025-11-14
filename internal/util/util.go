package util

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// parseSize parses a size string like "2G", "500M", "1.5G" to bytes
func ParseSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))
	if sizeStr == "" {
		return 0, fmt.Errorf("empty size string")
	}

	// Extract numeric part and unit
	var numStr string
	var unit string
	for i, ch := range sizeStr {
		if ch >= '0' && ch <= '9' || ch == '.' {
			numStr += string(ch)
		} else {
			unit = sizeStr[i:]
			break
		}
	}

	if numStr == "" {
		return 0, fmt.Errorf("no numeric value found")
	}

	value, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value: %w", err)
	}

	var multiplier int64
	switch unit {
	case "B", "":
		multiplier = 1
	case "K", "KB":
		multiplier = 1024
	case "M", "MB":
		multiplier = 1024 * 1024
	case "G", "GB":
		multiplier = 1024 * 1024 * 1024
	case "T", "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown unit: %s (use B, K/KB, M/MB, G/GB, T/TB)", unit)
	}

	return int64(value * float64(multiplier)), nil
}

func FormatBytesToHumanReadable(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(n)
	i := 0

	for v >= 1000 && i < len(units)-1 {
		v /= 1000
		i++
	}

	if i == 0 {
		return fmt.Sprintf("%d %s", int64(v), units[i])
	}
	return fmt.Sprintf("%.2f %s", v, units[i])
}

func SafeBase(name string) string {
	if name == "" {
		return "file"
	}
	return filepath.Base(name)
}
