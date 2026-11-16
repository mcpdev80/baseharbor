package logs

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

const alloyJournalContainerGID = 470

func hostSystemdJournalGID() (string, error) {
	if value := strings.TrimSpace(os.Getenv("BASEHARBOR_SYSTEMD_JOURNAL_GID")); value != "" {
		gid, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return "", fmt.Errorf("BASEHARBOR_SYSTEMD_JOURNAL_GID must be a numeric GID: %w", err)
		}
		return strconv.FormatUint(gid, 10), nil
	}
	group, err := user.LookupGroup("systemd-journal")
	if err != nil {
		return "", fmt.Errorf("resolve host systemd-journal group: %w", err)
	}
	if _, err := strconv.ParseUint(group.Gid, 10, 32); err != nil {
		return "", fmt.Errorf("host systemd-journal group has invalid GID %q: %w", group.Gid, err)
	}
	return group.Gid, nil
}

func journalGroupMappingForMode(mode string) (journalGroupMapping, error) {
	if mode != "journald" {
		return journalGroupMapping{}, nil
	}
	gid, err := hostSystemdJournalGID()
	if err != nil {
		return journalGroupMapping{}, err
	}
	return journalGroupMapping{HostGID: gid}, nil
}
