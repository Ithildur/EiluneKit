package openapi_test

import (
	"bytes"
	"encoding/json/v2"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/Ithildur/EiluneKit/tools/openapi"
	"github.com/getkin/kin-openapi/openapi3"
)

type decimalQuantity uint64

func (quantity decimalQuantity) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, strconv.FormatUint(uint64(quantity), 10)), nil
}

func (decimalQuantity) JSONSchemaAlias() any { return "" }

type quantityObject struct {
	Value string `json:"value"`
}

type objectQuantity uint64

func (quantity objectQuantity) MarshalJSON() ([]byte, error) {
	return json.Marshal(quantityObject{Value: strconv.FormatUint(uint64(quantity), 10)})
}

func (objectQuantity) JSONSchemaAlias() any { return quantityObject{} }

func TestGenerateSchemaAliases(t *testing.T) {
	type envelope struct {
		Quantity objectQuantity `json:"quantity"`
	}
	for _, test := range []struct {
		name   string
		schema routes.SchemaRef
		value  any
	}{
		{"root", routes.SchemaOf[objectQuantity]("Quantity"), objectQuantity(42)},
		{"nested", routes.SchemaOf[envelope]("Envelope"), envelope{Quantity: 42}},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, err := openapi.Generate([]routes.Route{{
				Method: http.MethodGet, Path: "/quantity", OperationID: "getQuantity",
				Responses: map[string]routes.Response{"200": {
					Description: "Quantity", Content: routes.Content{"application/json": test.schema},
				}},
			}}, openapi.Options{Title: "Quantity API", Version: "1"})
			if err != nil {
				t.Fatal(err)
			}
			doc, err := openapi3.NewLoader().LoadFromData(payload)
			if err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			var actual any
			if err := json.Unmarshal(wire, &actual); err != nil {
				t.Fatal(err)
			}
			schema := doc.Paths.Value("/quantity").Get.Responses.Value("200").Value.Content["application/json"].Schema.Value
			if err := schema.VisitJSON(actual); err != nil {
				t.Fatalf("schema alias rejects the real JSON payload %s: %v", wire, err)
			}
			if err := schema.VisitJSON("invalid"); err == nil {
				t.Fatal("object schema alias accepts a string")
			}
		})
	}
}

func TestGenerateResponseHeadersWithSchemaAlias(t *testing.T) {
	type usage struct {
		Bytes decimalQuantity `json:"bytes"`
		Count int             `json:"count"`
	}
	route := routes.Route{
		Method: http.MethodGet, Path: "/usage", OperationID: "getUsage",
		Responses: map[string]routes.Response{"200": {
			Description: "Usage",
			Content:     routes.Content{"application/json": routes.SchemaOf[usage]("Usage")},
			Headers: map[string]routes.Header{
				"X-Bytes":      {Required: true, Description: "Byte quantity", Schema: routes.SchemaOf[decimalQuantity]("DecimalQuantity")},
				"X-Request-ID": {Schema: routes.SchemaOf[string]("")},
			},
		}},
	}
	opts := openapi.Options{Title: "Usage API", Version: "1"}
	payload, err := openapi.Generate([]routes.Route{route}, opts)
	if err != nil {
		t.Fatal(err)
	}
	again, err := openapi.Generate([]routes.Route{route}, opts)
	if err != nil || !bytes.Equal(payload, again) {
		t.Fatalf("generation is not deterministic: %v", err)
	}
	doc, err := openapi3.NewLoader().LoadFromData(payload)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(usage{Bytes: decimalQuantity(^uint64(0)), Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]any
	if err := json.Unmarshal(wire, &actual); err != nil {
		t.Fatal(err)
	}
	response := doc.Paths.Value("/usage").Get.Responses.Value("200").Value
	schema := response.Content["application/json"].Schema.Value
	if err := schema.VisitJSON(actual); err != nil {
		t.Fatalf("generated schema rejects the real JSON payload %s: %v", wire, err)
	}
	if !schema.Properties["bytes"].Value.Type.Is("string") || !schema.Properties["count"].Value.Type.Is("integer") {
		t.Fatalf("custom or default type mapping is incorrect: %s", payload)
	}
	header := response.Headers["X-Bytes"].Value
	if !header.Required || header.Description != "Byte quantity" || header.Schema.Ref != "#/components/schemas/DecimalQuantity" || !header.Schema.Value.Type.Is("string") {
		t.Fatalf("response header contract was lost: %s", payload)
	}
	if !response.Headers["X-Request-ID"].Value.Schema.Value.Type.Is("string") {
		t.Fatal("default header mapping was lost")
	}
	copy := route.Clone()
	delete(copy.Responses["200"].Headers, "X-Bytes")
	if _, ok := route.Responses["200"].Headers["X-Bytes"]; !ok {
		t.Fatal("mutating the clone changed the original header contract")
	}
}

func TestGenerateRejectsInvalidResponseHeaders(t *testing.T) {
	header := routes.Header{Schema: routes.SchemaOf[string]("")}
	for _, test := range []struct {
		name    string
		headers map[string]routes.Header
		want    string
	}{
		{"duplicate", map[string]routes.Header{"X-ID": header, "x-id": header}, "duplicate response header"},
		{"invalid name", map[string]routes.Header{"X Bad": header}, "invalid response header name"},
		{"content type", map[string]routes.Header{"Content-Type": header}, "must use response content"},
		{"missing schema", map[string]routes.Header{"X-ID": {}}, "schema type is required"},
		{"conflicting component", map[string]routes.Header{
			"X-First":  {Schema: routes.SchemaOf[string]("HeaderValue")},
			"X-Second": {Schema: routes.SchemaOf[int]("HeaderValue")},
		}, "refers to both"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := openapi.Generate([]routes.Route{{
				Method: http.MethodGet, Path: "/headers", OperationID: "getHeaders",
				Responses: map[string]routes.Response{"204": {Description: "Headers", Headers: test.headers}},
			}}, openapi.Options{Title: "Headers", Version: "1"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}
