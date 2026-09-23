package jsonutil

import "testing"

func TestDecodeRejectsNonObjectsAndUnknownFields(t *testing.T) {
	for _, raw := range []string{"null", "[]", "", `{} {}`, `{"unexpected":true}`} {
		var value struct {
			Name string `json:"name"`
		}
		if err := DecodeObject([]byte(raw), &value); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	var value struct {
		Name string `json:"name"`
	}
	if err := DecodeObject([]byte(` {"name":"test"} `), &value); err != nil || value.Name != "test" {
		t.Fatal(value, err)
	}
}
