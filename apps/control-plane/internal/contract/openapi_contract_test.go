package contract_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/operation"
	"gopkg.in/yaml.v3"
)

type openAPIDocument struct {
	Info struct {
		Description string `yaml:"description"`
	} `yaml:"info"`
	Components struct {
		Schemas map[string]openAPISchema `yaml:"schemas"`
	} `yaml:"components"`
}

type openAPISchema struct {
	Required   []string                   `yaml:"required"`
	Properties map[string]openAPIProperty `yaml:"properties"`
}

type openAPIProperty struct {
	Ref     string   `yaml:"$ref"`
	Enum    []string `yaml:"enum"`
	Pattern string   `yaml:"pattern"`
}

func TestOperationDTOsMatchOpenAPI(t *testing.T) {
	document := loadOpenAPI(t)

	assertSchemaMatchesDTO(t, document, "Operation", reflect.TypeOf(operation.Operation{}))
	assertSchemaMatchesDTO(t, document, "ResourceReference", reflect.TypeOf(operation.ResourceRef{}))
	assertSchemaMatchesDTO(t, document, "OperationStep", reflect.TypeOf(operation.Step{}))
	assertSchemaMatchesDTO(t, document, "OperationError", reflect.TypeOf(operation.StructuredError{}))

	stepStatus, ok := reflect.TypeOf(operation.Step{}).FieldByName("Status")
	if !ok {
		t.Fatal("operation.Step.Status is missing")
	}
	if stepStatus.Type != reflect.TypeOf(operation.Status("")) {
		t.Fatalf("operation.Step.Status type = %v, want operation.Status", stepStatus.Type)
	}

	stepSchema := document.Components.Schemas["OperationStep"]
	if stepSchema.Properties["status"].Ref != "#/components/schemas/OperationStatus" {
		t.Fatalf("OperationStep.status ref = %q", stepSchema.Properties["status"].Ref)
	}
	operationSchema := document.Components.Schemas["Operation"]
	if operationSchema.Properties["target"].Ref != "#/components/schemas/ResourceReference" {
		t.Fatalf("Operation.target ref = %q", operationSchema.Properties["target"].Ref)
	}
	if operationSchema.Properties["error"].Ref != "#/components/schemas/OperationError" {
		t.Fatalf("Operation.error ref = %q", operationSchema.Properties["error"].Ref)
	}
}

func TestOpenAPIUsesVersionedContractAndStableProblemTypes(t *testing.T) {
	document := loadOpenAPI(t)
	if !strings.Contains(document.Info.Description, "initial versioned `/api/v1` surface") {
		t.Fatalf("OpenAPI description does not identify the initial versioned API surface")
	}

	problemSchema, ok := document.Components.Schemas["ProblemDetails"]
	if !ok {
		t.Fatal("ProblemDetails schema is missing")
	}
	const expectedPattern = "^urn:gpu-container-cloud:problem:[a-z0-9][a-z0-9._-]*$"
	if problemSchema.Properties["type"].Pattern != expectedPattern {
		t.Fatalf("ProblemDetails.type pattern = %q, want %q", problemSchema.Properties["type"].Pattern, expectedPattern)
	}

	dependencySchema, ok := document.Components.Schemas["DependencyCheck"]
	if !ok {
		t.Fatal("DependencyCheck schema is missing")
	}
	assertStringSet(t, "DependencyCheck.status enum", dependencySchema.Properties["status"].Enum, []string{"ready", "unavailable"})
}

func loadOpenAPI(t *testing.T) openAPIDocument {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve contract test path")
	}
	specPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "api", "openapi", "control-plane-v1.yaml")

	contents, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read OpenAPI contract %s: %v", specPath, err)
	}
	var document openAPIDocument
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	return document
}

func assertSchemaMatchesDTO(t *testing.T, document openAPIDocument, schemaName string, dtoType reflect.Type) {
	t.Helper()
	schema, ok := document.Components.Schemas[schemaName]
	if !ok {
		t.Fatalf("%s schema is missing", schemaName)
	}

	properties := make([]string, 0, len(schema.Properties))
	for property := range schema.Properties {
		properties = append(properties, property)
	}
	fields, required := jsonFields(dtoType)
	assertStringSet(t, schemaName+" properties", properties, fields)
	assertStringSet(t, schemaName+" required fields", schema.Required, required)
}

func jsonFields(dtoType reflect.Type) ([]string, []string) {
	fields := make([]string, 0, dtoType.NumField())
	required := make([]string, 0, dtoType.NumField())
	for index := 0; index < dtoType.NumField(); index++ {
		field := dtoType.Field(index)
		tag := field.Tag.Get("json")
		parts := strings.Split(tag, ",")
		name := parts[0]
		if name == "" || name == "-" {
			continue
		}
		fields = append(fields, name)
		if !containsString(parts[1:], "omitempty") {
			required = append(required, name)
		}
	}
	return fields, required
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func assertStringSet(t *testing.T, label string, actual, expected []string) {
	t.Helper()
	actual = append([]string(nil), actual...)
	expected = append([]string(nil), expected...)
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("%s = %v, want %v", label, actual, expected)
	}
}
