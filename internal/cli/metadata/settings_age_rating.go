package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared"
)

var ageRatingFields = []metadataSettingField{
	{name: "advertising", kind: metadataSettingBool},
	{name: "ageAssurance", kind: metadataSettingBool},
	{name: "gambling", kind: metadataSettingBool},
	{name: "healthOrWellnessTopics", kind: metadataSettingBool},
	{name: "lootBox", kind: metadataSettingBool},
	{name: "messagingAndChat", kind: metadataSettingBool},
	{name: "parentalControls", kind: metadataSettingBool},
	{name: "socialMedia", kind: metadataSettingBool},
	{name: "socialMediaAgeRestricted", kind: metadataSettingBool},
	{name: "unrestrictedWebAccess", kind: metadataSettingBool},
	{name: "userGeneratedContent", kind: metadataSettingBool},
	{name: "alcoholTobaccoOrDrugUseOrReferences"},
	{name: "contests"},
	{name: "gamblingSimulated"},
	{name: "gunsOrOtherWeapons"},
	{name: "horrorOrFearThemes"},
	{name: "matureOrSuggestiveThemes"},
	{name: "medicalOrTreatmentInformation"},
	{name: "profanityOrCrudeHumor"},
	{name: "sexualContentGraphicAndNudity"},
	{name: "sexualContentOrNudity"},
	{name: "violenceCartoonOrFantasy"},
	{name: "violenceRealistic"},
	{name: "violenceRealisticProlongedGraphicOrSadistic"},
	{name: "kidsAgeBand"},
	{name: "ageRatingOverride"},
	{name: "ageRatingOverrideV2"},
	{name: "koreaAgeRatingOverride"},
	{name: "gracRatingClassificationNumber"},
	{name: "developerAgeRatingInfoUrl"},
}

var ageRatingSettingScope = metadataSettingScope{
	name:         "age-rating",
	fileName:     "age-rating.json",
	fields:       ageRatingFields,
	needsAppInfo: true,
	fetch:        fetchAgeRatingSetting,
	update:       updateAgeRatingSetting,
}

func fetchAgeRatingDeclaration(ctx context.Context, client *asc.Client, appInfoID string) (*asc.AgeRatingDeclarationResponse, error) {
	return shared.RetryReadWithFreshTimeout(ctx, func(requestCtx context.Context) (*asc.AgeRatingDeclarationResponse, error) {
		return client.GetAgeRatingDeclarationForAppInfo(requestCtx, appInfoID)
	})
}

func fetchAgeRatingSetting(ctx context.Context, client *asc.Client, target metadataSettingTarget) (map[string]string, error) {
	resp, err := fetchAgeRatingDeclaration(ctx, client, target.appInfoID)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(resp.Data.Attributes)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, field := range ageRatingFields {
		rawValue, ok := raw[field.name]
		if !ok || isJSONNull(rawValue) {
			continue
		}
		if field.kind == metadataSettingBool {
			var value bool
			if err := json.Unmarshal(rawValue, &value); err != nil {
				return nil, fmt.Errorf("decode %s: %w", field.name, err)
			}
			values[field.name] = strconv.FormatBool(value)
			continue
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return nil, fmt.Errorf("decode %s: %w", field.name, err)
		}
		if value != "" {
			values[field.name] = value
		}
	}
	return values, nil
}

func updateAgeRatingSetting(ctx context.Context, client *asc.Client, target metadataSettingTarget, changed map[string]string) error {
	resp, err := fetchAgeRatingDeclaration(ctx, client, target.appInfoID)
	if err != nil {
		return err
	}
	data, err := encodeMetadataSettingFile(metadataSettingScope{fields: ageRatingFields}, changed)
	if err != nil {
		return err
	}
	var attributes asc.AgeRatingDeclarationAttributes
	if err := json.Unmarshal(data, &attributes); err != nil {
		return err
	}
	_, err = client.UpdateAgeRatingDeclaration(ctx, resp.Data.ID, attributes)
	return err
}
