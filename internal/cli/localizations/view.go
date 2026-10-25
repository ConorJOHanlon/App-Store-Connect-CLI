package localizations

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

// LocalizationsViewCommand returns the view localizations subcommand.
func LocalizationsViewCommand() *ffcli.Command {
	fs := flag.NewFlagSet("view", flag.ExitOnError)

	localizationID := fs.String("id", "", "Localization resource ID")
	locType := fs.String("type", shared.LocalizationTypeVersion, "Localization type: version (default) or app-info")
	fields := fs.String("fields", "", "Fields to return, comma-separated. version: "+strings.Join(versionLocalizationFieldList(), ", ")+"; app-info: "+strings.Join(appInfoLocalizationFieldList(), ", "))
	output := shared.BindOutputFlags(fs)

	return &ffcli.Command{
		Name:       "view",
		ShortUsage: "asc localizations view --id LOCALIZATION_ID [flags]",
		ShortHelp:  "View one localization by ID.",
		LongHelp: `View one App Store version or app info localization by ID.

JSON output is App Store Connect's response, unmodified.

Examples:
  asc localizations view --id "LOCALIZATION_ID"
  asc localizations view --id "LOCALIZATION_ID" --fields "description,whatsNew"
  asc localizations view --type app-info --id "LOCALIZATION_ID" --fields "name,subtitle"`,
		FlagSet:   fs,
		UsageFunc: shared.DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			normalizedType, err := shared.NormalizeLocalizationType(*locType)
			if err != nil {
				return shared.UsageError(err.Error())
			}
			resourceType, allowedFields := "appStoreVersionLocalizations", versionLocalizationFieldList()
			if normalizedType == shared.LocalizationTypeAppInfo {
				resourceType, allowedFields = "appInfoLocalizations", appInfoLocalizationFieldList()
			}
			fieldValues, err := shared.NormalizeSelection(*fields, allowedFields, "--fields")
			if err != nil {
				return shared.UsageError(err.Error())
			}
			fieldsProvided := false
			fs.Visit(func(f *flag.Flag) {
				fieldsProvided = fieldsProvided || f.Name == "fields"
			})
			if fieldsProvided && len(fieldValues) == 0 {
				return shared.UsageError("--fields must not be empty")
			}
			id := strings.TrimSpace(*localizationID)
			if id == "" {
				fmt.Fprintln(os.Stderr, "Error: --id is required")
				return shared.MissingRequiredUsageError("--id")
			}

			path := "/v1/" + resourceType + "/" + url.PathEscape(id)
			if len(fieldValues) > 0 {
				query := url.Values{}
				query.Set("fields["+resourceType+"]", strings.Join(fieldValues, ","))
				path += "?" + query.Encode()
			}

			client, err := shared.GetASCClient()
			if err != nil {
				return fmt.Errorf("localizations view: %w", err)
			}
			requestCtx, cancel := shared.ContextWithTimeout(ctx)
			defer cancel()

			response, err := client.RawRequest(requestCtx, "GET", path, nil)
			if err != nil {
				return fmt.Errorf("localizations view: %w", err)
			}

			render := func(write func([]string, [][]string) error) func() error {
				return func() error {
					headers, rows, err := localizationViewRows(response)
					if err != nil {
						return fmt.Errorf("localizations view: %w", err)
					}
					return write(headers, rows)
				}
			}
			return shared.PrintOutputWithRenderers(json.RawMessage(response), *output.Output, *output.Pretty, render(asc.WriteTable), render(asc.WriteMarkdown))
		},
	}
}

// localizationViewRows lists the ID and every attribute Apple returned, so
// table and markdown output show exactly the fields a --fields request asked for.
func localizationViewRows(response []byte) ([]string, [][]string, error) {
	var envelope struct {
		Data struct {
			ID         string                     `json:"id"`
			Attributes map[string]json.RawMessage `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}
	keys := make([]string, 0, len(envelope.Data.Attributes))
	for key := range envelope.Data.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := [][]string{{"id", envelope.Data.ID}}
	for _, key := range keys {
		raw := envelope.Data.Attributes[key]
		value := string(raw)
		var text string
		if json.Unmarshal(raw, &text) == nil {
			value = text
		} else if string(raw) == "null" {
			value = ""
		}
		rows = append(rows, []string{key, value})
	}
	return []string{"Field", "Value"}, rows, nil
}

// versionLocalizationFieldList reports the fields[appStoreVersionLocalizations]
// values the single-resource endpoint accepts.
func versionLocalizationFieldList() []string {
	return []string{"description", "locale", "keywords", "marketingUrl", "promotionalText", "supportUrl", "whatsNew", "appStoreVersion", "appScreenshotSets", "appPreviewSets", "searchKeywords", "placements"}
}

// appInfoLocalizationFieldList reports the fields[appInfoLocalizations] values
// the single-resource endpoint accepts.
func appInfoLocalizationFieldList() []string {
	return []string{"locale", "name", "subtitle", "privacyPolicyUrl", "privacyChoicesUrl", "privacyPolicyText", "appInfo"}
}
