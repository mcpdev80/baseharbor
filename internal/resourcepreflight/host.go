package resourcepreflight

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type MemoryPressure struct {
	SomeAvg10  float64 `json:"some_avg10,omitempty"`
	SomeAvg60  float64 `json:"some_avg60,omitempty"`
	SomeAvg300 float64 `json:"some_avg300,omitempty"`
	FullAvg10  float64 `json:"full_avg10,omitempty"`
	FullAvg60  float64 `json:"full_avg60,omitempty"`
	FullAvg300 float64 `json:"full_avg300,omitempty"`
}

type HostMemory struct {
	TotalBytes     uint64          `json:"total_bytes"`
	AvailableBytes uint64          `json:"available_bytes"`
	SwapTotalBytes uint64          `json:"swap_total_bytes"`
	SwapFreeBytes  uint64          `json:"swap_free_bytes"`
	Pressure       *MemoryPressure `json:"pressure,omitempty"`
}

func ReadLinuxHostMemory() (HostMemory, error) {
	meminfo, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return HostMemory{}, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	host, err := ParseMeminfo(meminfo)
	if err != nil {
		return HostMemory{}, err
	}
	if pressure, err := os.ReadFile("/proc/pressure/memory"); err == nil {
		if parsed, parseErr := ParseMemoryPressure(pressure); parseErr == nil {
			host.Pressure = &parsed
		}
	}
	return host, nil
}

func ParseMeminfo(data []byte) (HostMemory, error) {
	values := map[string]uint64{}
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		switch name {
		case "MemTotal", "MemAvailable", "SwapTotal", "SwapFree":
		default:
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return HostMemory{}, fmt.Errorf("parse %s: %w", name, err)
		}
		// Linux /proc/meminfo reports these values in KiB.
		values[name] = value * 1024
	}
	if err := s.Err(); err != nil {
		return HostMemory{}, err
	}
	if values["MemTotal"] == 0 || values["MemAvailable"] == 0 {
		return HostMemory{}, fmt.Errorf("meminfo does not contain usable MemTotal/MemAvailable")
	}
	return HostMemory{
		TotalBytes: values["MemTotal"],
		AvailableBytes: values["MemAvailable"],
		SwapTotalBytes: values["SwapTotal"],
		SwapFreeBytes: values["SwapFree"],
	}, nil
}

func ParseMemoryPressure(data []byte) (MemoryPressure, error) {
	var out MemoryPressure
	seen := false
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) == 0 {
			continue
		}
		target := ""
		switch fields[0] {
		case "some":
			target = "some"
		case "full":
			target = "full"
		default:
			continue
		}
		seen = true
		for _, field := range fields[1:] {
			key, raw, ok := strings.Cut(field, "=")
			if !ok || (key != "avg10" && key != "avg60" && key != "avg300") {
				continue
			}
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return MemoryPressure{}, fmt.Errorf("parse memory PSI %s: %w", key, err)
			}
			switch target + "." + key {
			case "some.avg10":
				out.SomeAvg10 = value
			case "some.avg60":
				out.SomeAvg60 = value
			case "some.avg300":
				out.SomeAvg300 = value
			case "full.avg10":
				out.FullAvg10 = value
			case "full.avg60":
				out.FullAvg60 = value
			case "full.avg300":
				out.FullAvg300 = value
			}
		}
	}
	if err := s.Err(); err != nil {
		return MemoryPressure{}, err
	}
	if !seen {
		return MemoryPressure{}, fmt.Errorf("memory PSI data contains no some/full rows")
	}
	return out, nil
}
