package testflight

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

// betaGroupReadPage fetches one page of a beta group's related resources.
// An empty groupID with a next URL follows that URL verbatim.
type betaGroupReadPage func(ctx context.Context, client *asc.Client, groupID, next string, limit int, fields []string) (asc.PaginatedResponse, error)

type betaGroupReadConfig struct {
	resource    string
	fieldsUsage string
	fieldValues []string
	fetch       betaGroupReadPage
}

// BetaGroupsBuildsCommand returns the beta-groups builds command group.
func BetaGroupsBuildsCommand() *ffcli.Command {
	return betaGroupReadGroupCommand(betaGroupReadConfig{
		resource:    "builds",
		fieldsUsage: "Fields to include for builds, comma-separated",
		fieldValues: betaGroupBuildFieldsValues,
		fetch: func(ctx context.Context, client *asc.Client, groupID, next string, limit int, fields []string) (asc.PaginatedResponse, error) {
			return client.GetBetaGroupBuilds(ctx, groupID,
				asc.WithBetaGroupBuildsLimit(limit),
				asc.WithBetaGroupBuildsNextURL(next),
				asc.WithBetaGroupBuildsFields(fields),
			)
		},
	})
}

// BetaGroupsTestersCommand returns the beta-groups testers command group.
func BetaGroupsTestersCommand() *ffcli.Command {
	return betaGroupReadGroupCommand(betaGroupReadConfig{
		resource:    "testers",
		fieldsUsage: "Fields to include for testers, comma-separated",
		fieldValues: betaGroupTesterFieldsValues,
		fetch: func(ctx context.Context, client *asc.Client, groupID, next string, limit int, fields []string) (asc.PaginatedResponse, error) {
			return client.GetBetaGroupTesters(ctx, groupID,
				asc.WithBetaGroupTestersLimit(limit),
				asc.WithBetaGroupTestersNextURL(next),
				asc.WithBetaGroupTestersFields(fields),
			)
		},
	})
}

func betaGroupReadGroupCommand(config betaGroupReadConfig) *ffcli.Command {
	fs := flag.NewFlagSet(config.resource, flag.ExitOnError)

	return &ffcli.Command{
		Name:       config.resource,
		ShortUsage: "asc testflight beta-groups " + config.resource + " <subcommand> [flags]",
		ShortHelp:  "List " + config.resource + " in a beta group.",
		LongHelp: `List ` + config.resource + ` in a beta group.

Examples:
  asc testflight beta-groups ` + config.resource + ` list --group-id "GROUP_ID"`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Subcommands: []*ffcli.Command{
			betaGroupReadListCommand(config),
		},
		Exec: func(ctx context.Context, args []string) error {
			return flag.ErrHelp
		},
	}
}

func betaGroupReadListCommand(config betaGroupReadConfig) *ffcli.Command {
	fs := flag.NewFlagSet(config.resource+" list", flag.ExitOnError)

	groupID := shared.BindResourceIDFlag(fs, "group-id", "betaGroups", "Beta group ID")
	fields := fs.String("fields", "", config.fieldsUsage+": "+strings.Join(config.fieldValues, ", "))
	limit := fs.Int("limit", 0, "Maximum results per page (1-200)")
	next := fs.String("next", "", "Fetch next page using a links.next URL")
	paginate := fs.Bool("paginate", false, "Automatically fetch all pages (aggregate results)")
	output := shared.BindOutputFlags(fs)

	command := "beta-groups " + config.resource + " list"
	// RejectNextFlagConflicts has no context to carry the tree's path
	// rewrites, so errors name the public path directly.
	errPrefix := "testflight groups " + config.resource + " list"
	return &ffcli.Command{
		Name:       "list",
		ShortUsage: "asc testflight " + command + " [flags]",
		ShortHelp:  "List " + config.resource + " in a beta group.",
		LongHelp: `List ` + config.resource + ` in a beta group by group ID; no --app is needed.

--group-id and --fields cannot be combined with --next: a links.next URL
already carries the query it was produced from.

Examples:
  asc testflight ` + command + ` --group-id "GROUP_ID"
  asc testflight ` + command + ` --group-id "GROUP_ID" --fields "` + strings.Join(config.fieldValues[:2], ",") + `"
  asc testflight ` + command + ` --group-id "GROUP_ID" --paginate`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			if *limit != 0 && (*limit < 1 || *limit > 200) {
				return shared.WithDiagnostic(
					shared.UsageErrorCtx(ctx, errPrefix+": --limit must be between 1 and 200"),
					shared.DiagnosticInvalidInput,
					"--limit",
				)
			}
			if err := shared.ValidateNextURL(*next); err != nil {
				return shared.UsageErrorfCtx(ctx, "%s: %v", errPrefix, err)
			}
			if err := shared.RejectNextFlagConflicts(fs, *next, errPrefix, "group-id", "fields"); err != nil {
				return err
			}
			fieldValues, err := shared.NormalizeSelection(*fields, config.fieldValues, "--fields")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
				return shared.InvalidValueUsageError("--fields")
			}
			groupValue := strings.TrimSpace(*groupID)
			nextValue := strings.TrimSpace(*next)
			if groupValue == "" && nextValue == "" {
				fmt.Fprintln(os.Stderr, "Error: --group-id is required")
				return shared.MissingRequiredUsageError("--group-id")
			}

			client, err := shared.GetASCClient()
			if err != nil {
				return fmt.Errorf("%s: %w", errPrefix, err)
			}

			requestCtx, cancel := shared.ContextWithTimeout(ctx)
			defer cancel()

			if *paginate {
				resp, err := shared.PaginateWithSpinner(
					requestCtx,
					func(ctx context.Context) (asc.PaginatedResponse, error) {
						return config.fetch(ctx, client, groupValue, nextValue, 200, fieldValues)
					},
					func(ctx context.Context, nextURL string) (asc.PaginatedResponse, error) {
						return config.fetch(ctx, client, "", nextURL, 0, nil)
					},
				)
				if err != nil {
					return fmt.Errorf("%s: %w", errPrefix, err)
				}
				return shared.PrintOutput(resp, *output.Output, *output.Pretty)
			}

			resp, err := config.fetch(requestCtx, client, groupValue, nextValue, *limit, fieldValues)
			if err != nil {
				return fmt.Errorf("%s: failed to fetch: %w", errPrefix, err)
			}
			return shared.PrintOutput(resp, *output.Output, *output.Pretty)
		},
	}
}
