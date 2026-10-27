package providerupgrade

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major int
	Minor int
	Patch int
}

func ParseVersion(value string) (Version, error) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "v"))
	if i := strings.IndexAny(value, "-+"); i >= 0 {
		value = value[:i]
	}
	parts := strings.Split(value, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Version{}, fmt.Errorf("unsupported version %q", value)
	}
	numbers := make([]int, 3)
	for i, part := range parts {
		if part == "" {
			return Version{}, fmt.Errorf("unsupported version %q", value)
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("unsupported version %q", value)
		}
		numbers[i] = n
	}
	return Version{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}, nil
}

func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

func (v Version) SameMinor(other Version) bool {
	return v.Major == other.Major && v.Minor == other.Minor
}
