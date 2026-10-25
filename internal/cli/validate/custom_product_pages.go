package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/validation"
)

const (
	customProductPageVersionReplaced = "REPLACED_WITH_NEW_VERSION"
	// includedSearchKeywordsLimit is the API maximum for limit[searchKeywords].
	includedSearchKeywordsLimit = 50
)

// fetchCustomProductPageKeywords lists the search keywords of every
// localization on the app's current custom product page versions.
func fetchCustomProductPageKeywords(ctx context.Context, client *asc.Client, appID string) ([]validation.CustomProductPageLocalization, error) {
	appID = strings.TrimSpace(appID)
	pages, err := fetchAllReadinessPages(ctx, func(requestCtx context.Context, nextURL string) (*asc.AppCustomProductPagesResponse, error) {
		return client.GetAppCustomProductPages(requestCtx, appID, asc.WithAppCustomProductPagesLimit(200), asc.WithAppCustomProductPagesNextURL(nextURL))
	})
	if err != nil || len(pages) == 0 {
		return nil, err
	}

	var mu sync.Mutex
	var localizations []validation.CustomProductPageLocalization
	tasks := make([]readinessTask, 0, len(pages))
	for _, page := range pages {
		tasks = append(tasks, func(taskCtx context.Context) error {
			versions, err := fetchAllReadinessPages(taskCtx, func(requestCtx context.Context, nextURL string) (*asc.AppCustomProductPageVersionsResponse, error) {
				return client.GetAppCustomProductPageVersions(requestCtx, page.ID, asc.WithAppCustomProductPageVersionsLimit(200), asc.WithAppCustomProductPageVersionsNextURL(nextURL))
			})
			if err != nil {
				return err
			}
			for _, version := range versions {
				if strings.EqualFold(strings.TrimSpace(version.Attributes.State), customProductPageVersionReplaced) {
					continue
				}
				locs, err := fetchAllReadinessPages(taskCtx, func(requestCtx context.Context, nextURL string) (*asc.AppCustomProductPageLocalizationsResponse, error) {
					return client.GetAppCustomProductPageLocalizations(
						requestCtx,
						version.ID,
						asc.WithAppCustomProductPageLocalizationsLimit(200),
						asc.WithAppCustomProductPageLocalizationsSearchKeywords(includedSearchKeywordsLimit),
						asc.WithAppCustomProductPageLocalizationsNextURL(nextURL),
					)
				})
				if err != nil {
					return err
				}
				for _, loc := range locs {
					entry := validation.CustomProductPageLocalization{
						PageName:       page.Attributes.Name,
						LocalizationID: loc.ID,
						Locale:         loc.Attributes.Locale,
					}
					keywords, complete := includedSearchKeywords(loc.Relationships)
					if !complete {
						fetched, err := fetchAllReadinessPages(taskCtx, func(requestCtx context.Context, nextURL string) (*asc.AppKeywordsResponse, error) {
							return client.GetAppCustomProductPageLocalizationSearchKeywords(requestCtx, loc.ID, asc.WithAppSearchKeywordsLimit(200), asc.WithAppSearchKeywordsNextURL(nextURL))
						})
						if err != nil {
							return err
						}
						for _, keyword := range fetched {
							keywords = append(keywords, keyword.ID)
						}
					}
					entry.SearchKeywords = keywords
					mu.Lock()
					localizations = append(localizations, entry)
					mu.Unlock()
				}
			}
			return nil
		})
	}
	if err := runReadinessTasks(ctx, tasks...); err != nil {
		return nil, err
	}

	sort.Slice(localizations, func(i, j int) bool {
		if localizations[i].PageName != localizations[j].PageName {
			return localizations[i].PageName < localizations[j].PageName
		}
		if localizations[i].Locale != localizations[j].Locale {
			return localizations[i].Locale < localizations[j].Locale
		}
		return localizations[i].LocalizationID < localizations[j].LocalizationID
	})
	return localizations, nil
}

// includedSearchKeywords reads the keywords included in a localization's
// searchKeywords relationship. complete is false when the relationship is
// missing or truncated, so the caller must fetch the keywords directly.
func includedSearchKeywords(relationships json.RawMessage) (keywords []string, complete bool) {
	var parsed struct {
		SearchKeywords *struct {
			Data []asc.RelationshipData `json:"data"`
			Meta struct {
				Paging struct {
					Total *int `json:"total"`
				} `json:"paging"`
			} `json:"meta"`
		} `json:"searchKeywords"`
	}
	if len(relationships) == 0 || json.Unmarshal(relationships, &parsed) != nil || parsed.SearchKeywords == nil {
		return nil, false
	}
	related := parsed.SearchKeywords
	total := related.Meta.Paging.Total
	if total == nil || *total > len(related.Data) {
		return nil, false
	}
	for _, keyword := range related.Data {
		keywords = append(keywords, keyword.ID)
	}
	return keywords, true
}

// fetchAllReadinessPages fetches every page of a list endpoint through the
// readiness request gate. fetch receives an empty nextURL for the first page.
func fetchAllReadinessPages[T any](ctx context.Context, fetch func(context.Context, string) (*asc.Response[T], error)) ([]asc.Resource[T], error) {
	firstPage, err := doReadinessRequest(ctx, func(requestCtx context.Context) (*asc.Response[T], error) {
		return fetch(requestCtx, "")
	})
	if err != nil {
		return nil, err
	}
	paginated, err := asc.PaginateAll(ctx, firstPage, func(_ context.Context, nextURL string) (asc.PaginatedResponse, error) {
		return doReadinessRequest(ctx, func(requestCtx context.Context) (asc.PaginatedResponse, error) {
			return fetch(requestCtx, nextURL)
		})
	})
	if err != nil {
		return nil, err
	}
	resp, ok := paginated.(*asc.Response[T])
	if !ok {
		return nil, fmt.Errorf("unexpected pagination response type: %T", paginated)
	}
	return resp.Data, nil
}
