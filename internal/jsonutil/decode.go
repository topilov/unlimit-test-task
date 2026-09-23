package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"apm-investigator/internal/incident"
)

func DecodeObject(raw []byte, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("%w: expected JSON object", incident.ErrInvalidInput)
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", incident.ErrInvalidInput, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON", incident.ErrInvalidInput)
	}
	return nil
}
