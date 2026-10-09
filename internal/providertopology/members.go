// Package providertopology binds provider member intent to retained native state.
package providertopology

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/availability"
	"go.yaml.in/yaml/v3"
)

// ResolveMembers never rewrites a retained datastore topology. Callers without
// an intent (maintenance/rotation) retain its actual members; fresh providers
// default to one. Manifest callers must pass their resolved component intent.
func ResolveMembers(compose, prefix string, recommended int, intent ...availability.Requirement) (int, error) {
	if len(intent) > 1 {
		return 0, errors.New("one provider availability intent required")
	}
	desired := 1
	if len(intent) == 1 {
		if intent[0].HA {
			desired = recommended
			if intent[0].Instances > 0 {
				desired = intent[0].Instances
			}
			if desired < 2 {
				return 0, errors.New("provider HA requires multiple members")
			}
		} else if intent[0].Instances > 1 {
			return 0, errors.New("multiple provider members require explicit ha: true")
		}
	}
	actual, err := ExistingMembers(compose, prefix)
	if errors.Is(err, os.ErrNotExist) {
		return desired, nil
	}
	if err != nil {
		return 0, err
	}
	if len(intent) == 1 && desired != actual {
		return 0, fmt.Errorf("provider topology change blocked: retained %s has %d data members, requested %d; preserve the existing intent or perform an explicitly supported data migration", prefix, actual, desired)
	}
	return actual, nil
}

func ExistingMembers(compose, prefix string) (int, error) {
	services, err := ServiceNames(compose)
	if err != nil {
		return 0, err
	}
	members := map[int]bool{}
	for service := range services {
		if !strings.HasPrefix(service, prefix+"-") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(service, prefix+"-"))
		if err == nil && n > 0 {
			members[n] = true
		}
	}
	if len(members) == 0 {
		return 0, errors.New("retained provider has no identifiable data members")
	}
	for n := 1; n <= len(members); n++ {
		if !members[n] {
			return 0, errors.New("retained provider member inventory is incomplete")
		}
	}
	return len(members), nil
}

// ServiceNames reads retained native state without following links or accepting
// an unprotected file. A missing file denotes a fresh installation.
func ServiceNames(compose string) (map[string]any, error) {
	info, err := os.Lstat(compose)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("provider topology requires protected regular Compose state")
	}
	data, err := os.ReadFile(compose)
	if err != nil {
		return nil, err
	}
	var document struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document.Services == nil {
		return nil, errors.New("retained provider has no service inventory")
	}
	return document.Services, nil
}

func Names(prefix string, count int) []string {
	var names []string
	for n := 1; n <= count; n++ {
		names = append(names, fmt.Sprintf("%s-%d", prefix, n))
	}
	return names
}

// RequireVariant protects providers whose native HA layout has several roles
// rather than a uniform member group.
func RequireVariant(compose, single, haMarker string, ha bool) error {
	services, err := ServiceNames(compose)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, retainedHA := services[haMarker]
	_, retainedSingle := services[single]
	if retainedHA == retainedSingle {
		return errors.New("retained provider storage layout is unverified")
	}
	if retainedHA != ha {
		return errors.New("provider topology change blocked: retained storage layout differs from explicit HA intent; use an explicitly supported data migration")
	}
	return nil
}
