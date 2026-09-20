package api

import "encoding/json"

func usageEventQoderCredits(value *string) json.RawMessage {
	if value == nil || !json.Valid([]byte(*value)) {
		return nil
	}
	return json.RawMessage(*value)
}
