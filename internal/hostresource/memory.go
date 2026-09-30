package hostresource

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Confidence string

const (
	ConfidenceExplicit      Confidence = "EXPLICIT"
	ConfidenceKnownBaseline Confidence = "KNOWN_BASELINE"
	ConfidenceEstimated     Confidence = "ESTIMATED"
	ConfidenceUnknown       Confidence = "UNKNOWN"
)

type Decision string

const (
	DecisionSafe   Decision = "SAFE"
	DecisionTight  Decision = "TIGHT"
	DecisionUnsafe Decision = "UNSAFE"
)

type Pressure struct {
	Available  bool    `json:"available"`
	SomeAvg300 float64 `json:"some_avg300,omitempty"`
	FullAvg300 float64 `json:"full_avg300,omitempty"`
}

type MemoryEvidence struct {
	TotalBytes     uint64   `json:"total_bytes"`
	AvailableBytes uint64   `json:"available_bytes"`
	SwapTotalBytes uint64   `json:"swap_total_bytes"`
	SwapFreeBytes  uint64   `json:"swap_free_bytes"`
	Pressure       Pressure `json:"pressure"`
}

type ComponentEstimate struct {
	Name           string     `json:"name"`
	MinimumBytes   uint64     `json:"minimum_bytes,omitempty"`
	EstimatedBytes uint64     `json:"estimated_bytes,omitempty"`
	Confidence     Confidence `json:"confidence"`
	Source         string     `json:"source,omitempty"`
}

type MemoryEstimate struct {
	MinimumBytes   uint64              `json:"minimum_bytes"`
	EstimatedBytes uint64              `json:"estimated_bytes"`
	Confidence     Confidence          `json:"confidence"`
	Components     []ComponentEstimate `json:"components,omitempty"`
}

type Policy struct {
	SafetyMarginBytes    uint64
	PressureSomeAvg300 float64
	PressureFullAvg300 float64
	SwapFreeRatioTight float64
}

func DefaultPolicy() Policy {
	return Policy{
		SafetyMarginBytes:   512 << 20,
		PressureSomeAvg300:  1.0,
		PressureFullAvg300:  0.5,
		SwapFreeRatioTight:  0.10,
	}
}

type Result struct {
	Evidence MemoryEvidence `json:"evidence"`
	Estimate MemoryEstimate `json:"estimate"`
	Decision Decision       `json:"decision"`
	Reasons  []string       `json:"reasons,omitempty"`
}

func Evaluate(evidence MemoryEvidence, estimate MemoryEstimate, policy Policy) Result {
	result := Result{Evidence: evidence, Estimate: estimate, Decision: DecisionSafe}

	if estimate.MinimumBytes > 0 && evidence.AvailableBytes < estimate.MinimumBytes {
		result.Decision = DecisionUnsafe
		result.Reasons = append(result.Reasons,
			fmt.Sprintf("available memory %d bytes is below reliable planned minimum %d bytes", evidence.AvailableBytes, estimate.MinimumBytes))
		return result
	}

	tight := false
	if estimate.EstimatedBytes > 0 {
		required := estimate.EstimatedBytes
		if ^uint64(0)-required < policy.SafetyMarginBytes {
			required = ^uint64(0)
		} else {
			required += policy.SafetyMarginBytes
		}
		if evidence.AvailableBytes < required {
			tight = true
			result.Reasons = append(result.Reasons,
				fmt.Sprintf("available memory %d bytes is below planned estimate plus safety margin %d bytes", evidence.AvailableBytes, required))
		}
	}

	if evidence.SwapTotalBytes > 0 {
		freeRatio := float64(evidence.SwapFreeBytes) / float64(evidence.SwapTotalBytes)
		if freeRatio <= policy.SwapFreeRatioTight {
			tight = true
			result.Reasons = append(result.Reasons, "swap is nearly exhausted")
		}
	}
	if evidence.Pressure.Available {
		if evidence.Pressure.SomeAvg300 >= policy.PressureSomeAvg300 {
			tight = true
			result.Reasons = append(result.Reasons, "sustained memory PSI pressure is elevated")
		}
		if evidence.Pressure.FullAvg300 >= policy.PressureFullAvg300 {
			tight = true
			result.Reasons = append(result.Reasons, "sustained full memory PSI pressure is elevated")
		}
	}
	if tight {
		result.Decision = DecisionTight
	}
	return result
}

func ReadLinux() (MemoryEvidence, error) {
	return readLinuxFiles("/proc/meminfo", "/proc/pressure/memory")
}

func readLinuxFiles(meminfoPath, pressurePath string) (MemoryEvidence, error) {
	meminfo, err := os.Open(meminfoPath)
	if err != nil {
		return MemoryEvidence{}, fmt.Errorf("read host memory evidence: %w", err)
	}
	defer meminfo.Close()

	values := map[string]uint64{}
	scanner := bufio.NewScanner(meminfo)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		switch key {
		case "MemTotal", "MemAvailable", "SwapTotal", "SwapFree":
			kib, parseErr := strconv.ParseUint(fields[1], 10, 64)
			if parseErr != nil {
				return MemoryEvidence{}, fmt.Errorf("parse %s from meminfo: %w", key, parseErr)
			}
			values[key] = kib * 1024
		}
	}
	if err := scanner.Err(); err != nil {
		return MemoryEvidence{}, fmt.Errorf("scan host memory evidence: %w", err)
	}
	if values["MemTotal"] == 0 || values["MemAvailable"] == 0 {
		return MemoryEvidence{}, errors.New("host memory evidence is missing MemTotal or MemAvailable")
	}

	evidence := MemoryEvidence{
		TotalBytes:     values["MemTotal"],
		AvailableBytes: values["MemAvailable"],
		SwapTotalBytes: values["SwapTotal"],
		SwapFreeBytes:  values["SwapFree"],
	}
	pressure, err := readPressure(pressurePath)
	if err == nil {
		evidence.Pressure = pressure
	}
	return evidence, nil
}

func readPressure(path string) (Pressure, error) {
	f, err := os.Open(path)
	if err != nil {
		return Pressure{}, err
	}
	defer f.Close()

	result := Pressure{Available: true}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		var target *float64
		switch fields[0] {
		case "some":
			target = &result.SomeAvg300
		case "full":
			target = &result.FullAvg300
		default:
			continue
		}
		for _, field := range fields[1:] {
			if !strings.HasPrefix(field, "avg300=") {
				continue
			}
			value, parseErr := strconv.ParseFloat(strings.TrimPrefix(field, "avg300="), 64)
			if parseErr != nil {
				return Pressure{}, parseErr
			}
			*target = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Pressure{}, err
	}
	return result, nil
}

func Sum(components []ComponentEstimate) MemoryEstimate {
	result := MemoryEstimate{Components: append([]ComponentEstimate(nil), components...)}
	rank := map[Confidence]int{
		ConfidenceExplicit:      3,
		ConfidenceKnownBaseline: 2,
		ConfidenceEstimated:     1,
		ConfidenceUnknown:       0,
	}
	result.Confidence = ConfidenceExplicit
	if len(components) == 0 {
		result.Confidence = ConfidenceUnknown
	}
	for _, component := range components {
		result.MinimumBytes += component.MinimumBytes
		result.EstimatedBytes += component.EstimatedBytes
		if rank[component.Confidence] < rank[result.Confidence] {
			result.Confidence = component.Confidence
		}
	}
	if result.EstimatedBytes < result.MinimumBytes {
		result.EstimatedBytes = result.MinimumBytes
	}
	return result
}
