package startup

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/backup"
	"github.com/webappsgo/redxt/src/cli"
	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/paths"
	"github.com/webappsgo/redxt/src/service"
)

// adminRecoveryTimeout bounds the two credential-clearing statements and
// the admin count query, which never touch more than one small table.
const adminRecoveryTimeout = 10 * time.Second

// serviceCommand runs the PART 24 `--service` subcommands. Every one of
// them installs, inspects, or drives the init system and then exits
// without starting a server.
func serviceCommand(opts *cli.Options, name string, streams IO) int {
	resolved := paths.ResolveWith(overridesFrom(opts))
	manager := service.New(serviceContext(resolved, name), paths.IsPrivileged(), nil)
	manager.Confirm = func(prompt string) bool {
		return confirm(prompt, streams)
	}

	args := append([]string{opts.Service}, opts.ServiceArgs...)
	if err := manager.Dispatch(args, streams.Out); err != nil {
		fmt.Fprintf(streams.Err, "service: %v\n", err)
		if strings.HasPrefix(err.Error(), "service: unknown subcommand") {
			return ExitUsage
		}
		return ExitFailure
	}
	return ExitOK
}

// serviceContext fills in the identifiers and resolved directories the
// unit-file templates need. Nothing here is hardcoded: the frozen
// namespace identifiers come from src/paths, which PART 4 makes their
// only home.
func serviceContext(resolved paths.Paths, name string) service.Context {
	return service.Context{
		ProjectName:  name,
		ProjectOrg:   paths.ProjectOrg(),
		InternalOrg:  paths.InternalOrg(),
		InternalName: paths.InternalName(),
		AppName:      config.DefaultApplicationName,
		BinaryPath:   resolved.Binary,
		ConfigDir:    resolved.Config,
		DataDir:      resolved.Data,
		CacheDir:     resolved.Cache,
		LogDir:       resolved.Logs,
		BackupDir:    resolved.Backup,
		PIDFile:      resolved.PIDFile,
	}
}

// confirm asks a yes/no question on the terminal and defaults to no, so
// an unattended run never proceeds through a destructive prompt.
func confirm(prompt string, streams IO) bool {
	fmt.Fprintf(streams.Out, "%s [y/N] ", prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// maintenanceCommand runs the PART 24 `--maintenance` subcommands:
// backup, restore, update, mode, and setup.
func maintenanceCommand(ctx context.Context, opts *cli.Options, name string, streams IO) int {
	resolved := paths.ResolveWith(overridesFrom(opts))

	switch opts.Maintenance {
	case "help", "--help", "-h":
		fmt.Fprint(streams.Out, cli.MaintenanceHelp(name, resolved.Backup))
		return ExitOK
	case "update":
		return maintenanceUpdate(ctx, opts, name, streams)
	}

	cfg, err := config.Load(resolved)
	if err != nil {
		fmt.Fprintf(streams.Err, "maintenance: %v\n", err)
		return ExitFailure
	}

	switch opts.Maintenance {
	case "backup":
		return maintenanceBackup(ctx, opts, cfg, resolved, streams)
	case "restore":
		return maintenanceRestore(ctx, opts, cfg, resolved, streams)
	case "mode":
		return maintenanceMode(opts, cfg, streams)
	case "setup":
		return maintenanceSetup(ctx, cfg, resolved, streams)
	}

	fmt.Fprintf(streams.Err, "maintenance: unknown command %q\n", opts.Maintenance)
	fmt.Fprint(streams.Err, cli.MaintenanceHelp(name, resolved.Backup))
	return ExitUsage
}

// maintenanceUpdate forwards `--maintenance update [cmd]` to the same
// implementation `--update [cmd]` uses, which PART 24 documents as two
// spellings of one command rather than two behaviors.
func maintenanceUpdate(ctx context.Context, opts *cli.Options, name string, streams IO) int {
	forwarded := *opts
	forwarded.Update = "check"
	forwarded.UpdateArgs = nil
	if len(opts.MaintenanceArgs) > 0 {
		forwarded.Update = opts.MaintenanceArgs[0]
		forwarded.UpdateArgs = opts.MaintenanceArgs[1:]
	}
	return updateCommand(ctx, &forwarded, name, streams)
}

// maintenanceBackup implements `--maintenance backup [file]
// [--password X]`.
func maintenanceBackup(ctx context.Context, opts *cli.Options, cfg *config.Config, resolved paths.Paths, streams IO) int {
	filename, password := splitMaintenanceArgs(opts.MaintenanceArgs)
	if password == "" && (cfg.Server.Backup.Encryption.Enabled || cfg.Server.Backup.Compliance.Enabled) {
		password = promptPassword("Enter backup password: ", streams)
	}

	svc := backupService(ctx, cfg, resolved, nil)
	result, err := svc.CreateManual(backup.CreateOptions{
		Filename:    filename,
		Password:    password,
		IncludeSSL:  true,
		IncludeData: true,
		CreatedBy:   currentActor(),
	})
	if err != nil {
		fmt.Fprintf(streams.Err, "backup: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(streams.Out, "Backup created: %s (%d bytes)\n", result.Path, result.SizeBytes)
	return ExitOK
}

// maintenanceRestore implements `--maintenance restore <file>
// [--password X]`.
func maintenanceRestore(ctx context.Context, opts *cli.Options, cfg *config.Config, resolved paths.Paths, streams IO) int {
	file, password := splitMaintenanceArgs(opts.MaintenanceArgs)
	if file == "" {
		fmt.Fprintln(streams.Err, "restore: a backup file is required")
		return ExitUsage
	}
	if password == "" && strings.HasSuffix(file, ".enc") {
		password = promptPassword("Enter backup password: ", streams)
	}

	empty, err := databaseIsEmpty(ctx, cfg, resolved)
	if err != nil {
		fmt.Fprintf(streams.Err, "restore: %v\n", err)
		return ExitFailure
	}

	svc := backupService(ctx, cfg, resolved, nil)
	result, err := svc.Restore(backup.RestoreOptions{
		BackupPath: file,
		Password:   password,
		Auth: backup.RestoreAuthContext{
			DatabaseEmpty: empty,
			IsRoot:        paths.IsPrivileged(),
		},
		RestoredBy: currentActor(),
	})
	if err != nil {
		fmt.Fprintf(streams.Err, "restore: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(streams.Out, "Restored from %s (created %s)\n", file, result.Manifest.CreatedAt.Format(time.RFC3339))
	if result.NewServer && result.SetupToken != "" {
		fmt.Fprintf(streams.Out, "Primary Admin must be re-created. Setup token: %s\n", result.SetupToken)
	}
	return ExitOK
}

// maintenanceMode implements `--maintenance mode <mode>`. PART 5's
// authorization table allows root outright; anyone else is refused here
// rather than silently rewriting the file.
func maintenanceMode(opts *cli.Options, cfg *config.Config, streams IO) int {
	if len(opts.MaintenanceArgs) == 0 {
		fmt.Fprintln(streams.Err, "mode: a mode is required (production or development)")
		return ExitUsage
	}
	if !paths.IsPrivileged() {
		fmt.Fprintln(streams.Err, "mode: changing the application mode requires root")
		return ExitFailure
	}
	if err := config.SetMode(cfg, opts.MaintenanceArgs[0]); err != nil {
		fmt.Fprintf(streams.Err, "mode: %v\n", err)
		return ExitUsage
	}
	fmt.Fprintf(streams.Out, "Application mode set to %s\n", cfg.Server.Mode)
	return ExitOK
}

// maintenanceSetup implements `--maintenance setup`: it clears the
// Server Admin credentials and prints a one-time setup token that the
// first-run wizard accepts.
func maintenanceSetup(ctx context.Context, cfg *config.Config, resolved paths.Paths, streams IO) int {
	usersDB, err := database.OpenUsers(cfg.Server.Database, resolved.DB)
	if err != nil {
		fmt.Fprintf(streams.Err, "setup: %v\n", err)
		return ExitFailure
	}
	defer usersDB.Close()

	if err := database.EnsureUsersSchema(ctx, usersDB); err != nil {
		fmt.Fprintf(streams.Err, "setup: %v\n", err)
		return ExitFailure
	}

	empty, err := adminsTableEmpty(ctx, usersDB)
	if err != nil {
		fmt.Fprintf(streams.Err, "setup: %v\n", err)
		return ExitFailure
	}

	svc := backupService(ctx, cfg, resolved, &adminRecovery{ctx: ctx, db: usersDB})
	token, err := svc.RunSetup(backup.SetupAuthContext{
		DatabaseEmpty: empty,
		IsRoot:        paths.IsPrivileged(),
	}, currentActor())
	if err != nil {
		fmt.Fprintf(streams.Err, "setup: %v\n", err)
		return ExitFailure
	}
	fmt.Fprintf(streams.Out, "Server Admin credentials cleared. Setup token: %s\n", token)
	return ExitOK
}

// backupService assembles the PART 22 service from the configuration and
// the resolved paths. Audit and Recorder stay nil on the command-line
// path: both are optional, and neither has a server to attach to when
// the binary was invoked purely to make or restore a backup.
func backupService(_ context.Context, cfg *config.Config, resolved paths.Paths, admin backup.AdminRecovery) *backup.Service {
	return &backup.Service{
		Project:   paths.ProjectName(),
		Paths:     resolved,
		Config:    cfg.Server.Backup,
		Retention: cfg.Server.Scheduler.Tasks["backup_daily"].Retention,
		Admin:     admin,
	}
}

// adminRecovery is the concrete AdminRecovery the backup package leaves
// to its caller: it clears every Server Admin password hash and revokes
// every admin API token, and touches nothing else.
type adminRecovery struct {
	ctx context.Context
	db  *database.DB
}

// ClearAdminCredentials performs the two statements PART 22's admin
// recovery command specifies.
func (a *adminRecovery) ClearAdminCredentials() error {
	if _, err := database.ExecContext(a.ctx, a.db, adminRecoveryTimeout,
		`UPDATE admins SET password_hash = ''`); err != nil {
		return err
	}
	_, err := database.ExecContext(a.ctx, a.db, adminRecoveryTimeout,
		`DELETE FROM api_tokens WHERE owner_type = 'admin'`)
	return err
}

// adminsTableEmpty reports whether users.db holds no Server Admin yet,
// which is the "empty database" branch of PART 22's authorization table.
func adminsTableEmpty(ctx context.Context, db *database.DB) (bool, error) {
	count := 0
	err := database.QueryRowContext(ctx, db, adminRecoveryTimeout, func(row *sql.Row) error {
		return row.Scan(&count)
	}, `SELECT COUNT(*) FROM admins`)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// databaseIsEmpty reports whether the server has any admin to protect,
// which decides whether a restore may proceed without credentials.
func databaseIsEmpty(ctx context.Context, cfg *config.Config, resolved paths.Paths) (bool, error) {
	usersDB, err := database.OpenUsers(cfg.Server.Database, resolved.DB)
	if err != nil {
		return false, err
	}
	defer usersDB.Close()

	if err := database.EnsureUsersSchema(ctx, usersDB); err != nil {
		return false, err
	}
	return adminsTableEmpty(ctx, usersDB)
}

// splitMaintenanceArgs separates the optional positional file argument
// from the `--password X` / `--password=X` form PART 22 documents.
func splitMaintenanceArgs(args []string) (file, password string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--password" || arg == "-password":
			if i+1 < len(args) {
				password = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--password="):
			password = strings.TrimPrefix(arg, "--password=")
		case strings.HasPrefix(arg, "-password="):
			password = strings.TrimPrefix(arg, "-password=")
		case file == "":
			file = arg
		}
	}
	return file, password
}

// promptPassword reads a backup password from the terminal. The value is
// never echoed back, logged, or written anywhere by this function.
func promptPassword(prompt string, streams IO) string {
	fmt.Fprint(streams.Out, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimRight(line, "\r\n")
}

// currentActor names the operating-system account that ran the command,
// which is what the backup audit trail records as the actor.
func currentActor() string {
	if name := os.Getenv("USER"); name != "" {
		return name
	}
	if name := os.Getenv("USERNAME"); name != "" {
		return name
	}
	return "cli"
}
