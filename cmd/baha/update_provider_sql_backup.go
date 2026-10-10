package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	Runtime         bhruntime.RuntimeProvider
	Project         string
	Compose         string
	Env             string
	Client          string
	Host            string
	CAFile          string
	User            string
	Password        string
	RestoreUser     string
	RestorePassword string
	Database        string
	Directory       string
	Name            string
	InstallationID  string
	Target          string
	Transaction     string
	Provider        providerupgrade.Provider
	ConfigPaths     []string
}

func (s providerSQLBackupSpec) validate() error {
	if (s.RestoreUser == "") != (s.RestorePassword == "") {
		return errors.New("provider SQL operator recovery identity is incomplete")
	}
	if s.Runtime == nil || s.Project == "" || s.Compose == "" || s.Env == "" || s.Client == "" ||
		s.Host == "" || s.CAFile == "" || s.User == "" || s.Password == "" || s.Database == "" ||
		s.Directory == "" || s.Name == "" || s.InstallationID == "" || s.Target == "" || s.Transaction == "" || s.Provider == "" {
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

func (s providerSQLBackupSpec) artifactBinding(version string) (string, error) {
	if version == "" || len(s.ConfigPaths) == 0 {
		return "", errors.New("provider backup has no version or configuration identity")
	}
	h := sha256.New()
	for _, value := range []string{string(s.Provider), s.Project, s.Compose, s.Env, s.Host, s.Database, s.Name, s.InstallationID, s.Transaction, version} {
		if value == "" || strings.ContainsRune(value, '\x00') {
			return "", errors.New("invalid provider backup identity field")
		}
		_, _ = io.WriteString(h, value+"\x00")
	}
	if s.Target == "" || strings.ContainsRune(s.Target, '\x00') {
		return "", errors.New("invalid provider recovery target")
	}
	_, _ = io.WriteString(h, s.Target+"\x00")
	for _, path := range s.ConfigPaths {
		if !filepath.IsAbs(path) {
			return "", errors.New("unbound provider backup configuration path")
		}
		_, _ = io.WriteString(h, path+"\x00")
	}
	for _, point := range []coreupdate.StreamRecoveryPoint{s.streamPoint(), s.configPoint()} {
		file, err := point.OpenVerified()
		if err != nil {
			return "", err
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		_, _ = h.Write(digest.Sum(nil))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// recoveryPairComplete avoids mixing a SQL snapshot from an interrupted
// transaction with freshly captured configuration from a later runtime state.
func (s providerSQLBackupSpec) recoveryPairComplete() (bool, error) {
	files := []string{
		filepath.Join(s.Directory, s.Name+"-sql.backup"),
		filepath.Join(s.Directory, s.Name+"-sql.backup.sha256"),
		filepath.Join(s.Directory, s.Name+"-config.backup"),
		filepath.Join(s.Directory, s.Name+"-config.backup.sha256"),
	}
	present := 0
	for _, path := range files {
		st, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return false, errors.New("unsafe provider recovery artifact")
		}
		present++
	}
	if present != 0 && present != len(files) {
		return false, errors.New("incomplete SQL/configuration backup pair from interrupted capture")
	}
	return present == len(files), nil
}

func (s providerSQLBackupSpec) capture(ctx context.Context, version string) (providerupgrade.BackupRef, error) {
	if err := s.validate(); err != nil {
		return providerupgrade.BackupRef{}, err
	}
	complete, err := s.recoveryPairComplete()
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	if complete {
		// Journal resume must never overwrite the existing SQL/configuration
		// pair with a snapshot of an already partially upgraded provider.
		binding, err := s.artifactBinding(version)
		if err != nil {
			return providerupgrade.BackupRef{}, err
		}
		ref := providerupgrade.BackupRef{Provider: s.Provider, ID: s.Name, Version: version, CreatedAt: time.Now().UTC(), Verified: true,
			Metadata: map[string]string{"database_verified": "true", "configuration_verified": "true", "format": "pg_dump-custom-v1", "binding": binding}}
		if err := s.verify(ctx, ref); err != nil {
			return providerupgrade.BackupRef{}, err
		}
		return ref, nil
	}
	environment, err := s.environment()
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	if err := s.streamPoint().Capture(ctx, func(ctx context.Context, dest io.Writer) error {
		const script = `IFS= read -r PGPASSWORD || exit 1
export PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT="$3" PGCONNECT_TIMEOUT=10
exec pg_dump --format=custom --compress=6 -h "$1" -U "$2" -d "$4"`
		return s.Runtime.RunProjectFilesEnv(ctx, s.Project, filepath.Dir(s.Compose), environment,
			strings.NewReader(s.Password+"\n"), dest, io.Discard, []string{s.Compose},
			"run", "--rm", "--no-deps", "-T", s.Client, "sh", "-ec", script, "--", s.Host, s.User, s.CAFile, s.Database)
	}); err != nil {
		return providerupgrade.BackupRef{}, fmt.Errorf("%s SQL backup: %w", s.Provider, err)
	}
	if err := s.captureConfiguration(ctx); err != nil {
		return providerupgrade.BackupRef{}, fmt.Errorf("%s configuration backup: %w", s.Provider, err)
	}
	binding, err := s.artifactBinding(version)
	if err != nil {
		return providerupgrade.BackupRef{}, err
	}
	ref := providerupgrade.BackupRef{
		Provider: s.Provider, ID: s.Name, Version: version, CreatedAt: time.Now().UTC(), Verified: true,
		Metadata: map[string]string{"database_verified": "true", "configuration_verified": "true", "format": "pg_dump-custom-v1", "binding": binding},
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
	binding, err := s.artifactBinding(ref.Version)
	if err != nil {
		return err
	}
	if ref.Metadata["binding"] != binding {
		return errors.New("provider SQL/configuration archive identity binding mismatch")
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
		"run", "--rm", "--no-deps", "-T", s.Client, "pg_restore", "--list"); err != nil {
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
		if count >= len(s.ConfigPaths) || header.Name != strconv.Itoa(count) || header.Size < 0 || header.Size > 4<<20 || header.Typeflag != tar.TypeReg || header.Mode&0022 != 0 {
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

// restoreConfiguration replays only the original, explicitly bound file list.
// Verify the entire archive before replacing any file. Each replacement is
// written privately and atomically renamed, so an interrupted run can replay
// the same verified archive without trusting current provider configuration.
func (s providerSQLBackupSpec) restoreConfiguration(ctx context.Context) error {
	if err := s.verifyConfigurationArchive(); err != nil {
		return err
	}
	archive, err := s.configPoint().OpenVerified()
	if err != nil {
		return err
	}
	defer archive.Close()
	tr := tar.NewReader(archive)
	type entry struct {
		path string
		data []byte
		mode os.FileMode
	}
	entries := make([]entry, 0, len(s.ConfigPaths))
	for i, path := range s.ConfigPaths {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if err != nil {
			return fmt.Errorf("missing provider configuration entry %d: %w", i, err)
		}
		if header.Name != strconv.Itoa(i) || header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > 4<<20 || header.Mode&0022 != 0 {
			return errors.New("unsafe provider configuration archive member")
		}
		if !filepath.IsAbs(path) {
			return errors.New("provider configuration destination must be absolute")
		}
		st, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0022 != 0 {
			return errors.New("foreign provider configuration destination")
		}
		data, err := io.ReadAll(io.LimitReader(tr, 4<<20+1))
		if err != nil {
			return err
		}
		if int64(len(data)) != header.Size {
			return errors.New("incomplete provider configuration content")
		}
		entries = append(entries, entry{path: path, data: data, mode: os.FileMode(header.Mode) & 0777})
	}
	if _, err := tr.Next(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing provider configuration entry")
	}
	for _, item := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		st, err := os.Lstat(item.path)
		if err != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0022 != 0 {
			return errors.New("provider configuration destination changed during recovery")
		}
		current, err := os.ReadFile(item.path)
		if err != nil {
			return err
		}
		if bytes.Equal(item.data, current) && st.Mode().Perm() == item.mode {
			continue
		}
		tmp, err := os.CreateTemp(filepath.Dir(item.path), ".provider-recover-*")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		if err := tmp.Chmod(item.mode); err != nil {
			tmp.Close()
			return err
		}
		if _, err := tmp.Write(item.data); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Sync(); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), item.path); err != nil {
			return err
		}
		d, err := os.Open(filepath.Dir(item.path))
		if err != nil {
			return err
		}
		err = d.Sync()
		d.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s providerSQLBackupSpec) restore(ctx context.Context, ref providerupgrade.BackupRef) error {
	if err := s.verify(ctx, ref); err != nil {
		return err
	}
	receipt := providerRestoreReceipt{Path: filepath.Join(s.Directory, s.Name+"-restore.json"), Binding: ref.Metadata["binding"]}
	phase, err := receipt.load()
	if err != nil {
		return err
	}
	if phase == "sql_started" || phase == "sql_failed" {
		return errors.New("BLOCKED: interrupted SQL restore has an unknown commit outcome; reconcile the database before replay")
	}
	if phase == "recovered" {
		return nil
	}
	if phase == "sql_restored" {
		if err := s.restoreConfiguration(ctx); err != nil {
			return err
		}
		return receipt.record("sql_restored", "recovered")
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
	// A migrated provider can add foreign keys/views absent from the original
	// archive. pg_restore --clean cannot drop their referenced old objects.
	// Decode into an owner-only temporary file, then replace only this bound
	// database's non-system schemas and restore ownership/ACLs in ONE transaction.
	// The schema reset already removes old objects; archive SQL must not issue
	// a second cleanup against schemas/tables that no longer exist.
	// Decode errors occur before the SQL-started receipt or any database mutation.
	sql, err := os.CreateTemp(s.Directory, ".provider-restore-*.sql")
	if err != nil {
		return err
	}
	defer func() { sql.Close(); os.Remove(sql.Name()) }()
	var diagnostics bytes.Buffer
	if err := s.Runtime.RunProjectFilesEnv(ctx, s.Project, filepath.Dir(s.Compose), environment,
		archive, sql, &diagnostics, []string{s.Compose}, "run", "--rm", "--no-deps", "-T", s.Client,
		"pg_restore", "--exit-on-error", "--file=-"); err != nil {
		return fmt.Errorf("%s SQL recovery decode failed (%s)", s.Provider, classifyProviderRestoreFailure(diagnostics.String()))
	}
	if _, err := sql.Seek(0, io.SeekStart); err != nil {
		return err
	}
	initializePublic, err := providerArchiveNeedsDefaultPublicSchema(sql)
	if err != nil {
		return err
	}
	if _, err := sql.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := receipt.record(phase, "sql_started"); err != nil {
		return err
	}
	// Restore the complete provider database, including its bootstrap-owned
	// extensions, with the retained private operator identity. Preserve archive
	// ownership/ACLs so provider and application roles never gain admin rights.
	user, password := s.User, s.Password
	if s.RestoreUser != "" {
		user, password = s.RestoreUser, s.RestorePassword
	}
	const resetOwnedSchemas = `DO $baseharbor_recovery$
DECLARE owned_schema text;
BEGIN
  FOR owned_schema IN SELECT nspname FROM pg_namespace
    WHERE nspname <> 'information_schema' AND nspname !~ '^pg_'
  LOOP
    EXECUTE format('DROP SCHEMA %I CASCADE', owned_schema);
  END LOOP;
END;
$baseharbor_recovery$;
`
	initialSchema := ""
	if initializePublic {
		// pg_dump intentionally omits CREATE for initdb's built-in public
		// schema. The archive still restores its original ownership and ACLs.
		initialSchema = "CREATE SCHEMA public AUTHORIZATION pg_database_owner;\n"
	}
	input := io.MultiReader(strings.NewReader(password+"\n"+resetOwnedSchemas+initialSchema), sql)
	diagnostics.Reset()
	const script = `IFS= read -r PGPASSWORD || exit 1
export PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT="$3" PGCONNECT_TIMEOUT=10
exec psql --no-psqlrc --single-transaction --set=ON_ERROR_STOP=1 --file=- -h "$1" -U "$2" -d "$4"`
	if err := s.Runtime.RunProjectFilesEnv(ctx, s.Project, filepath.Dir(s.Compose), environment,
		input, io.Discard, &diagnostics, []string{s.Compose},
		"run", "--rm", "--no-deps", "-T", s.Client, "sh", "-ec", script, "--", s.Host, user, s.CAFile, s.Database); err != nil {
		// Even an error can follow an accepted server COMMIT. Keep sql_started
		// until the database outcome is reconciled; never blindly repeat SQL.
		return fmt.Errorf("%s SQL recovery outcome requires reconciliation (%s): %w", s.Provider, classifyProviderRestoreFailure(diagnostics.String()), err)
	}
	if err := receipt.record("sql_started", "sql_restored"); err != nil {
		return err
	}
	if err := s.restoreConfiguration(ctx); err != nil {
		return fmt.Errorf("%s configuration recovery after SQL restore: %w", s.Provider, err)
	}
	return receipt.record("sql_restored", "recovered")
}

// Return only fixed error classes; native database diagnostics can contain
// protected row values and must never enter operator output or receipts.
func classifyProviderRestoreFailure(diagnostics string) string {
	for _, class := range []string{"must be owner of extension", "must be owner of schema", "must be owner of table", "permission denied", "unsupported version", "input file does not appear to be a valid archive", "could not read from input file", "connection refused", "database system is starting up", "other objects depend on it", "cannot drop", "already exists", "does not exist", "unexpected end of file", "authentication failed", "no password supplied", "no pg_hba.conf entry", "unrecognized configuration parameter", "could not execute query", "could not open input file", "could not read from input file", "could not connect to server"} {
		if strings.Contains(strings.ToLower(diagnostics), class) {
			return class
		}
	}
	return "native restore failed; protected diagnostics withheld"
}

// Inspect pg_restore's public-schema TOC definition before any table data.
// Recreated/custom public schemas have CREATE in the archive and must not be
// pre-created. Built-in public schemas rely on initdb; empty older archives can
// omit that entry entirely. No SQL/data is exposed in diagnostics.
func providerArchiveNeedsDefaultPublicSchema(sql io.Reader) (bool, error) {
	reader := bufio.NewReader(sql)
	publicDefinition := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return false, fmt.Errorf("provider archive schema inspection failed")
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-- Data for Name:") {
			return true, nil
		}
		if strings.HasPrefix(line, "-- Name:") {
			publicDefinition = strings.HasPrefix(line, "-- Name: public; Type: SCHEMA; Schema: -; Owner:")
		}
		if publicDefinition {
			if line == "-- *not* creating schema, since initdb creates it" {
				return true, nil
			}
			if line == "CREATE SCHEMA public;" || line == `CREATE SCHEMA "public";` {
				return false, nil
			}
		}
		if err == io.EOF {
			return true, nil
		}
	}
}
