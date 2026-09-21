package service

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

func normalizeUpstreamResponseModel(raw json.RawMessage) string {
	var model string
	if json.Unmarshal(raw, &model) != nil {
		return ""
	}
	model = strings.TrimSpace(model)
	if len(model) == 0 || len(model) > 512 || !utf8.ValidString(model) {
		return ""
	}
	for _, char := range model {
		if unicode.IsControl(char) {
			return ""
		}
	}
	return model
}
