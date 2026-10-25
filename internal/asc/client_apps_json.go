package asc

import (
	"bytes"
	"encoding/json"
)

// rawAttributes keeps the attribute object Apple returned so read output can
// carry absent, null, and unknown attributes that the typed struct drops.
type rawAttributes struct {
	original map[string]json.RawMessage
	decoded  map[string]json.RawMessage
}

func captureRawAttributes(data []byte, decoded any) (rawAttributes, error) {
	var original map[string]json.RawMessage
	if err := json.Unmarshal(data, &original); err != nil {
		return rawAttributes{}, err
	}
	baseline, err := json.Marshal(decoded)
	if err != nil {
		return rawAttributes{}, err
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(baseline, &values); err != nil {
		return rawAttributes{}, err
	}
	return rawAttributes{original: original, decoded: values}, nil
}

// merge overlays caller edits in encoded onto the original attributes. Values
// constructed directly, without a decode, keep their encoded shape.
func (r rawAttributes) merge(encoded []byte) ([]byte, error) {
	if r.decoded == nil {
		return encoded, nil
	}
	var current map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &current); err != nil {
		return nil, err
	}
	result := make(map[string]json.RawMessage, len(r.original))
	for name, value := range r.original {
		result[name] = value
	}
	for name, value := range current {
		if !bytes.Equal(value, r.decoded[name]) {
			result[name] = value
		}
	}
	for name := range r.decoded {
		if _, present := current[name]; !present {
			delete(result, name)
		}
	}
	return json.Marshal(result)
}

// UnmarshalJSON keeps the original attribute shape for sparse app responses.
func (a *AppAttributes) UnmarshalJSON(data []byte) error {
	type alias AppAttributes
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	raw, err := captureRawAttributes(data, decoded)
	if err != nil {
		return err
	}
	*a = AppAttributes(decoded)
	a.raw = raw
	return nil
}

// MarshalJSON preserves absent, null, and unknown API attributes while retaining
// caller edits and the existing shape of directly constructed AppAttributes.
func (a AppAttributes) MarshalJSON() ([]byte, error) {
	type alias AppAttributes
	encoded, err := json.Marshal(alias(a))
	if err != nil {
		return nil, err
	}
	return a.raw.merge(encoded)
}
