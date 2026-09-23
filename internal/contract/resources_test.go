package contract

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"apm-investigator/internal/incident"
)

func TestEmbeddedStrictContracts(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Prompt, "untrusted evidence") {
		t.Fatal("prompt lost trust boundary")
	}
	validateStrict(t, r.ResultSchema, r.ResultSchema)
	validateStrict(t, r.ReflectionSchema, r.ReflectionSchema)
	assertFields(t, r.ReflectionSchema, reflect.TypeFor[incident.Reflection]())
	lesson := r.ReflectionSchema["properties"].(map[string]any)["lesson"].(map[string]any)["anyOf"].([]any)[1].(map[string]any)
	assertFields(t, lesson, reflect.TypeFor[incident.LessonDraft]())
	assertFields(t, r.ResultSchema, reflect.TypeFor[incident.Result]())
	defs := r.ResultSchema["$defs"].(map[string]any)
	assertEnum(t, defs, "owner", incident.Owners())
	assertEnum(t, defs, "action", incident.Actions())
	assertEnum(t, defs, "claim", incident.Claims())
	assertFields(t, defs["hypotheses"].(map[string]any)["items"].(map[string]any), reflect.TypeFor[incident.Hypothesis]())
	assertFields(t, defs["open_questions"].(map[string]any)["items"].(map[string]any), reflect.TypeFor[incident.OpenQuestion]())
	names := []string{}
	for _, tool := range r.Tools {
		names = append(names, tool.Name)
		validateStrict(t, tool.Parameters, tool.Parameters)
		args := tool.Parameters["properties"].(map[string]any)["arguments"].(map[string]any)
		if tool.Name == incident.ToolCallbacks {
			ids := args["properties"].(map[string]any)["payment_ids"].(map[string]any)
			if ids["maxItems"] != float64(incident.MaxCallbackIDs) || ids["minItems"] != float64(1) {
				t.Fatal("callback bounds differ from executor")
			}
		}
	}
	want := incident.ToolNames()
	slices.Sort(want)
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Fatalf("tool allowlist drift: %v", names)
	}
}

func TestEvidenceReferencesContainOnlyIDs(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	refs := []map[string]any{
		r.ResultSchema["properties"].(map[string]any)["evidence_ids"].(map[string]any),
		r.ReflectionSchema["properties"].(map[string]any)["lesson"].(map[string]any)["anyOf"].([]any)[1].(map[string]any)["properties"].(map[string]any)["evidence_ids"].(map[string]any),
	}
	for _, tool := range r.Tools {
		defs := tool.Parameters["$defs"].(map[string]any)
		hypotheses := defs["hypotheses"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		refs = append(refs, hypotheses["supporting_evidence"].(map[string]any), hypotheses["contradicting_evidence"].(map[string]any))
	}
	for _, ref := range refs {
		pattern, ok := ref["items"].(map[string]any)["pattern"].(string)
		if !ok {
			t.Fatal("evidence reference has no pattern")
		}
		id := regexp.MustCompile(pattern)
		if !id.MatchString("E003") || id.MatchString("E003 shows no delivery attempts") {
			t.Fatalf("invalid evidence reference pattern %q", pattern)
		}
	}
}

func validateStrict(t *testing.T, node, root map[string]any) {
	t.Helper()
	if ref, ok := node["$ref"].(string); ok {
		if !strings.HasPrefix(ref, "#/$defs/") {
			t.Fatalf("unexpected ref %s", ref)
		}
		defs := root["$defs"].(map[string]any)
		if _, ok := defs[strings.TrimPrefix(ref, "#/$defs/")]; !ok {
			t.Fatalf("unresolved ref %s", ref)
		}
	}
	if node["type"] == "object" {
		props := node["properties"].(map[string]any)
		required := stringsFrom(node["required"])
		if node["additionalProperties"] != false || len(required) != len(props) {
			t.Fatal("non-strict object", node)
		}
		for key := range props {
			if !slices.Contains(required, key) {
				t.Fatalf("optional field %s", key)
			}
		}
	}
	for key, value := range node {
		if variants, ok := value.([]any); ok {
			for _, variant := range variants {
				if object, ok := variant.(map[string]any); ok {
					validateStrict(t, object, root)
				}
			}
		}
		if object, ok := value.(map[string]any); ok {
			if key == "properties" || key == "$defs" {
				for _, child := range object {
					validateStrict(t, child.(map[string]any), root)
				}
			} else {
				validateStrict(t, object, root)
			}
		}
	}
}
func assertFields(t *testing.T, schema map[string]any, goType reflect.Type) {
	t.Helper()
	props := schema["properties"].(map[string]any)
	if len(props) != goType.NumField() {
		t.Fatalf("%s field count differs from schema", goType)
	}
	for n := 0; n < goType.NumField(); n++ {
		tag := goType.Field(n).Tag.Get("json")
		if _, ok := props[tag]; !ok {
			t.Fatalf("%s.%s missing from schema", goType, tag)
		}
	}
}
func stringsFrom(v any) []string {
	values := v.([]any)
	out := make([]string, len(values))
	for n, value := range values {
		out[n] = value.(string)
	}
	return out
}

func assertEnum(t *testing.T, defs map[string]any, key string, values any) {
	t.Helper()
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	if err = json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	got := stringsFrom(defs[key].(map[string]any)["enum"])
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("%s enum drift: %v vs %v", key, got, want)
	}
}
