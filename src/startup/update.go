package startup

import (
	"context"
	"fmt"

	"github.com/webappsgo/redxt/src/cli"
	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/paths"
	"github.com/webappsgo/redxt/src/update"
)

// checkForUpdate is the scheduler handler for PART 19's update_check
// task. It honors server.update.defer_days, so the scheduled check only
// sees a release once it has aged past the operator's soak window, and
// it installs only when server.update.auto_install says so.
func (s *Server) checkForUpdate(ctx context.Context) error {
	cfg := s.Config.Server.Update
	return update.NewClient().CheckForUpdate(ctx, version.Version(), cfg.Branch, cfg.DeferDays,
		cfg.AutoInstall, s.Paths.Binary, paths.ProjectName(),
		func(release *update.Release) {
			s.Log.Infof("Update available: %s (branch %s)", release.TagName, cfg.Branch)
		}, s.Log)
}

// updateCommand runs the PART 8 --update subcommands, which print and
// exit without starting a server.
func updateCommand(ctx context.Context, opts *cli.Options, name string, streams IO) int {
	resolved := paths.ResolveWith(overridesFrom(opts))
	cfg, err := config.Load(resolved)
	if err != nil {
		fmt.Fprintf(streams.Err, "update: %v\n", err)
		return ExitFailure
	}

	client := update.NewClient()
	current := version.Version()
	branch := cfg.Server.Update.Branch

	switch opts.Update {
	case "help", "--help", "-h":
		latest := ""
		if release, err := client.CheckLatest(ctx, current, branch); err == nil && release != nil {
			latest = release.TagName
		}
		fmt.Fprint(streams.Out, cli.UpdateHelp(name, current, branch, latest))
		return ExitOK

	case "check":
		release, err := client.CheckLatest(ctx, current, branch)
		if err != nil {
			fmt.Fprintf(streams.Err, "update: %v\n", err)
			return ExitFailure
		}
		if release == nil {
			fmt.Fprintf(streams.Out, "%s %s is up to date (branch %s)\n", name, current, branch)
			return ExitOK
		}
		fmt.Fprintf(streams.Out, "Update available: %s (current %s, branch %s)\n", release.TagName, current, branch)
		fmt.Fprintf(streams.Out, "Run '%s --update yes' to install it.\n", name)
		return ExitOK

	case "yes":
		release, err := client.Update(ctx, current, branch, resolved.Binary, paths.ProjectName())
		if err != nil {
			fmt.Fprintf(streams.Err, "update: %v\n", err)
			return ExitFailure
		}
		if release == nil {
			fmt.Fprintf(streams.Out, "%s %s is up to date (branch %s)\n", name, current, branch)
			return ExitOK
		}
		fmt.Fprintf(streams.Out, "Updated to %s\n", release.TagName)
		return ExitOK

	case "branch":
		if len(opts.UpdateArgs) == 0 {
			fmt.Fprintf(streams.Err, "update: branch requires a name (stable, beta, or daily)\n")
			return ExitUsage
		}
		if err := update.SetBranch(cfg, opts.UpdateArgs[0]); err != nil {
			fmt.Fprintf(streams.Err, "update: %v\n", err)
			return ExitUsage
		}
		fmt.Fprintf(streams.Out, "Update branch set to %s\n", opts.UpdateArgs[0])
		return ExitOK
	}

	fmt.Fprintf(streams.Err, "update: unknown command %q\n", opts.Update)
	fmt.Fprint(streams.Err, cli.UpdateHelp(name, current, branch, ""))
	return ExitUsage
}
