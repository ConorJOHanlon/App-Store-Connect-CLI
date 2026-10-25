package asc

import (
	"bytes"
	"encoding/json"
	"strings"
)

// apiAttributes remembers the attributes object Apple returned so read output
// keeps null, false, empty, and unmodeled values that omitempty struct fields
// would drop. Values built in code have no snapshot and encode as before.
type apiAttributes struct {
	original map[string]json.RawMessage
	decoded  map[string]json.RawMessage
}

// unmarshalAPIAttributes decodes data into T, a method-free alias of an
// attributes type, and snapshots Apple's original object.
func unmarshalAPIAttributes[T any](data []byte) (T, apiAttributes, error) {
	var decoded T
	if err := json.Unmarshal(data, &decoded); err != nil {
		return decoded, apiAttributes{}, err
	}
	var original map[string]json.RawMessage
	if err := json.Unmarshal(data, &original); err != nil || original == nil {
		return decoded, apiAttributes{}, err
	}
	baseline, err := json.Marshal(decoded)
	if err != nil {
		return decoded, apiAttributes{}, err
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(baseline, &values); err != nil {
		return decoded, apiAttributes{}, err
	}
	return decoded, apiAttributes{original: original, decoded: values}, nil
}

// marshal encodes value, a method-free alias of an attributes type, over
// Apple's original object, applying only the fields changed in code.
func (a apiAttributes) marshal(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil || a.decoded == nil {
		return encoded, err
	}
	var current map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &current); err != nil {
		return nil, err
	}
	result := make(map[string]json.RawMessage, len(a.original))
	for name, value := range a.original {
		result[name] = value
	}
	for name, value := range current {
		if !bytes.Equal(value, a.decoded[name]) {
			deleteFoldedAttribute(result, name)
			result[name] = value
		}
	}
	for name := range a.decoded {
		if _, present := current[name]; !present {
			deleteFoldedAttribute(result, name)
		}
	}
	return json.Marshal(result)
}

// deleteFoldedAttribute removes every key encoding/json matches to name, so a
// changed value such as a redacted secret cannot survive under a differently
// cased duplicate key.
func deleteFoldedAttribute(attributes map[string]json.RawMessage, name string) {
	for key := range attributes {
		if strings.EqualFold(key, name) {
			delete(attributes, key)
		}
	}
}

func (a *AppStoreReviewDetailAttributes) UnmarshalJSON(data []byte) error {
	type alias AppStoreReviewDetailAttributes
	decoded, api, err := unmarshalAPIAttributes[alias](data)
	if err != nil {
		return err
	}
	*a = AppStoreReviewDetailAttributes(decoded)
	a.api = api
	return nil
}

func (a AppStoreReviewDetailAttributes) MarshalJSON() ([]byte, error) {
	type alias AppStoreReviewDetailAttributes
	return a.api.marshal(alias(a))
}

func (a *AppStoreVersionLocalizationAttributes) UnmarshalJSON(data []byte) error {
	type alias AppStoreVersionLocalizationAttributes
	decoded, api, err := unmarshalAPIAttributes[alias](data)
	if err != nil {
		return err
	}
	*a = AppStoreVersionLocalizationAttributes(decoded)
	a.api = api
	return nil
}

func (a AppStoreVersionLocalizationAttributes) MarshalJSON() ([]byte, error) {
	type alias AppStoreVersionLocalizationAttributes
	return a.api.marshal(alias(a))
}

func (a *AppInfoLocalizationAttributes) UnmarshalJSON(data []byte) error {
	type alias AppInfoLocalizationAttributes
	decoded, api, err := unmarshalAPIAttributes[alias](data)
	if err != nil {
		return err
	}
	*a = AppInfoLocalizationAttributes(decoded)
	a.api = api
	return nil
}

func (a AppInfoLocalizationAttributes) MarshalJSON() ([]byte, error) {
	type alias AppInfoLocalizationAttributes
	return a.api.marshal(alias(a))
}

func (a *BetaBuildLocalizationAttributes) UnmarshalJSON(data []byte) error {
	type alias BetaBuildLocalizationAttributes
	decoded, api, err := unmarshalAPIAttributes[alias](data)
	if err != nil {
		return err
	}
	*a = BetaBuildLocalizationAttributes(decoded)
	a.api = api
	return nil
}

func (a BetaBuildLocalizationAttributes) MarshalJSON() ([]byte, error) {
	type alias BetaBuildLocalizationAttributes
	return a.api.marshal(alias(a))
}

func (a *AgeRatingDeclarationAttributes) UnmarshalJSON(data []byte) error {
	type alias AgeRatingDeclarationAttributes
	decoded, api, err := unmarshalAPIAttributes[alias](data)
	if err != nil {
		return err
	}
	*a = AgeRatingDeclarationAttributes(decoded)
	a.api = api
	return nil
}

func (a AgeRatingDeclarationAttributes) MarshalJSON() ([]byte, error) {
	type alias AgeRatingDeclarationAttributes
	return a.api.marshal(alias(a))
}

func (a *InAppPurchaseAppStoreReviewScreenshotAttributes) UnmarshalJSON(data []byte) error {
	type alias InAppPurchaseAppStoreReviewScreenshotAttributes
	decoded, api, err := unmarshalAPIAttributes[alias](data)
	if err != nil {
		return err
	}
	*a = InAppPurchaseAppStoreReviewScreenshotAttributes(decoded)
	a.api = api
	return nil
}

func (a InAppPurchaseAppStoreReviewScreenshotAttributes) MarshalJSON() ([]byte, error) {
	type alias InAppPurchaseAppStoreReviewScreenshotAttributes
	return a.api.marshal(alias(a))
}
