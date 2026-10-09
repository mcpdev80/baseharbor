package main

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type providerSQLBackupSpec struct {
	Runtime     bhruntime.RuntimeProvider
	Project     string
	Compose     string
	Env         string
	Client      string
	Host        string
	CAFile      string
	User        string
	Password    string
	Database    string
	Directory   string
	Name        string
	Provider    providerupgrade.Provider
	ConfigPaths []string
}

func (s providerSQLBackupSpec) validate() error {
	if s.Runtime == nil || s.Project == "" || s.Compose == "" || s.Env == "" || s.Client == "" ||
		s.Host == "" || s.CAFile == "" || s.User == "" || s.Password == "" || s.Database == "" ||
		s.Directory == "" || s.Name == "" || s.Provider == "" {
		return errors.New("provider SQL backup requires owned runtime, authenticated database identity and private destination")
	}
	return nil
}

func (s providerSQLBackupSpec) environment() (map[string]string, error) {
	return bhruntime.RuntimeEnvironment(bhruntime.Files{Project: s.Project, Compose: s.Compose, Env: s.Env})
}

func (s providerSQLBackupSpec) streamPoint() coreupdate.StreamRecoveryPoint {
	return coreupdate.StreamRecoveryPoint{Directory: s.Directory, Name: s.Name + "-sql"}
}

func (s providerSQLBackupSpec) configPoint() coreupdate.StreamRecoveryPoint {
	return coreupdate.StreamRecoveryPoint{Directory: s.Directory, Name: s.Name + "-config"}
}

func (s providerSQLBackupSpec) capture(ctx context.Context, version string) (providerupgrade.BackupRef, error) {
	if err := s.validate(); err != nil {
		return providerupgrade.BackupRef{}, err
	}
	environment, err := s.environment()
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	if err := s.streamPoint().Capture(ctx, func(ctx context.Context, dest io.Writer) error {
		const script = `IFS= read -r PGPASSWORD || exit 1
export PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT="$3" PGCONNECT_TIMEOUT=10
exec pg_dump --format=custom --compress=6 --no-owner --no-acl -h "$1" -U "$2" -d "$4"`
		return s.Runtime.RunProjectFilesEnv(ctx, s.Project, filepath.Dir(s.Compose), environment,
			strings.NewReader(s.Password+"\n"), dest, io.Discard, []string{s.Compose},
			"run", "--rm", "--no-deps", s.Client, "sh", "-ec", script, "--", s.Host, s.User, s.CAFile, s.Database)
	}); err != nil {
		return providerupgrade.BackupRef{}, fmt.Errorf("%s SQL backup: %w", s.Provider, err)
	}
	if err := s.captureConfiguration(ctx); err != nil {
		return providerupgrade.BackupRef{}, fmt.Errorf("%s configuration backup: %w", s.Provider, err)
	}
	ref := providerupgrade.BackupRef{
		Provider: s.Provider, ID: s.Name, Version: version, CreatedAt: time.Now().UTC(), Verified: true,
		Metadata: map[string]string{"database_verified": "true", "configuration_verified": "true", "format": "pg_dump-custom-v1"},
	}
	if err := s.verify(ctx, ref); err != nil {
		return providerupgrade.BackupRef{}, err
	}
	return ref, nil
}

func (s providerSQLBackupSpec) captureConfiguration(ctx context.Context) error {
	if len(s.ConfigPaths) == 0 {
		return errors.New("provider configuration backup paths are required")
	}
	return s.configPoint().Capture(ctx, func(ctx context.Context, dest io.Writer) error {
		tw := tar.NewWriter(dest)
		for index, path := range s.ConfigPaths {
			if err := ctx.Err(); err != nil {
				_ = tw.Close()
				return err
			}
			st, err := os.Lstat(path)
			if err != nil {
				_ = tw.Close()
				return err
			}
			if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0022 != 0 {
				_ = tw.Close()
				return fmt.Errorf("unsafe provider configuration file %s", filepath.Base(path))
			}
			if st.Size() > 4<<20 {
				_ = tw.Close()
				return fmt.Errorf("provider configuration file %s is unexpectedly large", filepath.Base(path))
			}
			fd, err := os.Open(path)
			if err != nil {
				_ = tw.Close()
				return err
			}
			header := &tar.Header{Name: strconv.Itoa(index), Mode: int64(st.Mode().Perm()), Size: st.Size(), ModTime: time.Unix(0, 0)}
			if err := tw.WriteHeader(header); err != nil {
				fd.Close()
				_ = tw.Close()
				return err
			}
			_, copyErr := io.Copy(tw, fd)
			closeErr := fd.Close()
			if copyErr != nil {
				_ = tw.Close()
				return copyErr
			}
			if closeErr != nil {
				_ = tw.Close()
				return closeErr
			}
		}
		return tw.Close()
	})
}

func (s providerSQLBackupSpec) verify(ctx context.Context, ref providerupgrade.BackupRef) error {
	if err := s.validate(); err != nil {
		return err
	}
	if ref.Provider != s.Provider || ref.ID != s.Name || !ref.Verified ||
		ref.Metadata["database_verified"] != "true" || ref.Metadata["configuration_verified"] != "true" ||
		ref.Metadata["format"] != "pg_dump-custom-v1" {
		return errors.New("provider backup reference does not match owned SQL/configuration recovery artifacts")
	}
	if err := s.streamPoint().Verify(); err != nil {
		return err
	}
	if err := s.configPoint().Verify(); err != nil {
		return err
	}
	environment, err := s.environment()
	if err != nil {
		return err
	}
	archive, err := s.streamPoint().OpenVerified()
	if err != nil {
		return err
	}
	defer archive.Close()
	if err := s.Runtime.RunProjectFilesEnv(ctx, s.Project, filepath.Dir(s.Compose), environment,
		archive, io.Discard, io.Discard, []string{s.Compose},
		"run", "--rm", "--no-deps", s.Client, "pg_restore", "--list", "-"); err != nil {
		return fmt.Errorf("%s SQL archive structure verification failed: %w", s.Provider, err)
	}
	return s.verifyConfigurationArchive()
}

func (s providerSQLBackupSpec) verifyConfigurationArchive() error {
	archive, err := s.configPoint().OpenVerified()
	if err != nil {
		return err
	}
	defer archive.Close()
	tr := tar.NewReader(archive)
	count := 0
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if count >= len(s.ConfigPaths) || header.Name != strconv.Itoa(count) || header.Size < 0 || header.Size > 4<<20 {
			return errors.New("provider configuration archive structure changed")
		}
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return err
		}
		count++
	}
	if count != len(s.ConfigPaths) {
		return errors.New("provider configuration archive is incomplete")
	}
	return nil
}

func (s providerSQLBackupSpec) restore(ctx context.Context, ref providerupgrade.BackupRef) error {
	if err := s.verify(ctx, ref); err != nil {
		return err
	}
	environment, err := s.environment()
	if err != nil {
		return err
	}
	archive, err := s.streamPoint().OpenVerified()
	if err != nil {
		return err
	}
	defer archive.Close()
	input := io.MultiReader(strings.NewReader(s.Password+"\n"), archive)
	const script = `IFS= read -r PGPASSWORD || exit 1
export PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT="$3" PGCONNECT_TIMEOUT=10
exec pg_restore --clean --if-exists --no-owner --no-acl --exit-on-error -h "$1" -U "$2" -d "$4" -`
	if err := s.Runtime.RunProjectFilesEnv(ctx, s.Project, filepath.Dir(s.Compose), environment,
		input, io.Discard, io.Discard, []string{s.Compose},
		"run", "--rm", "--no-deps", s.Client, "sh", "-ec", script, "--", s.Host, s.User, s.CAFile, s.Database); err != nil {
		return fmt.Errorf("%s SQL recovery failed: %w", s.Provider, err)
	}
	return nil
}
