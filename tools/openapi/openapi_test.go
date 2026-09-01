package openapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/Ithildur/EiluneKit/tools/openapi"
)

type updateWidgetRequest struct {
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
}

type widgetResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func TestGenerateOpenAPI31Contract(t *testing.T) {
	api := routes.NewBlueprint()
	api.Put(
		"/widgets/{widgetID:[0-9]+}",
		"Update a widget",
		routes.Func(func(http.ResponseWriter, *http.Request, string) {}),
		routes.OperationID("updateWidget"),
		routes.Parameters(
			routes.Parameter{
				Name:     "widgetID",
				In:       routes.ParameterPath,
				Required: true,
				Schema:   routes.SchemaOf[int64](""),
			},
			routes.Parameter{
				Name:   "widgetID",
				In:     routes.ParameterQuery,
				Schema: routes.SchemaOf[bool](""),
			},
		),
		routes.JSONBody(routes.SchemaOf[updateWidgetRequest]("UpdateWidgetRequest"), true),
		routes.JSONResponse(http.StatusOK, "Updated widget", routes.SchemaOf[widgetResponse]("Widget")),
		routes.EmptyResponse(http.StatusNoContent, "Widget was unchanged"),
		routes.JSONResponse(http.StatusBadRequest, "Invalid request", routes.SchemaOf[response.ErrorResponse]("Error")),
		routes.Auth(routes.AuthRequired),
		routes.Security(routes.SecurityRequirement{
			{
				Name:         "BearerAuth",
				Type:         routes.SecurityHTTP,
				Scheme:       "bearer",
				BearerFormat: "JWT",
			},
		}),
	)

	payload, err := openapi.Generate(api.RoutesAt("/api"), openapi.Options{
		Title:   "Widget API",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("generate OpenAPI: %v", err)
	}
	again, err := openapi.Generate(api.RoutesAt("/api"), openapi.Options{
		Title:   "Widget API",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("generate OpenAPI again: %v", err)
	}
	if !bytes.Equal(payload, again) {
		t.Fatal("OpenAPI output is not deterministic")
	}

	var doc struct {
		OpenAPI string `json:"openapi"`
		Paths   map[string]map[string]struct {
			OperationID string `json:"operationId"`
			Parameters  []struct {
				Name   string `json:"name"`
				In     string `json:"in"`
				Schema struct {
					Type    string `json:"type"`
					Pattern string `json:"pattern"`
				} `json:"schema"`
			} `json:"parameters"`
			RequestBody struct {
				Content map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
			Responses map[string]any `json:"responses"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Required []string `json:"required"`
			} `json:"schemas"`
			SecuritySchemes map[string]any `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatalf("decode OpenAPI: %v", err)
	}
	if doc.OpenAPI != "3.1.1" {
		t.Fatalf("expected OpenAPI 3.1.1, got %q", doc.OpenAPI)
	}
	operation, ok := doc.Paths["/api/widgets/{widgetID}"]["put"]
	if !ok {
		t.Fatalf("normalized operation is missing: %s", payload)
	}
	if operation.OperationID != "updateWidget" {
		t.Fatalf("expected operation ID updateWidget, got %q", operation.OperationID)
	}
	if got := operation.Parameters[0].Schema.Type; got != "integer" {
		t.Fatalf("expected integer path parameter, got %q", got)
	}
	if got := operation.Parameters[0].Schema.Pattern; got != "" {
		t.Fatalf("path parameter inherited chi pattern %q", got)
	}
	requestSchema := operation.RequestBody.Content["application/json"].Schema.Ref
	if requestSchema != "#/components/schemas/UpdateWidgetRequest" {
		t.Fatalf("unexpected request schema ref %q", requestSchema)
	}
	for _, status := range []string{"200", "204", "400"} {
		if _, ok := operation.Responses[status]; !ok {
			t.Fatalf("response %s is missing", status)
		}
	}
	required := doc.Components.Schemas["UpdateWidgetRequest"].Required
	if !slices.Contains(required, "name") || slices.Contains(required, "note") {
		t.Fatalf("unexpected required request fields: %#v", required)
	}
	if _, ok := doc.Components.SecuritySchemes["BearerAuth"]; !ok {
		t.Fatal("BearerAuth security scheme is missing")
	}
}

func TestGenerateRejectsContractDrift(t *testing.T) {
	valid := routes.Route{
		Method:      http.MethodGet,
		Path:        "/widgets",
		OperationID: "listWidgets",
		Responses: map[string]routes.Response{
			"204": {Description: "No widgets"},
		},
	}

	tests := []struct {
		name   string
		routes []routes.Route
		want   string
	}{
		{
			name: "duplicate operation ID",
			routes: []routes.Route{
				valid,
				func() routes.Route {
					route := valid.Clone()
					route.Path = "/other"
					return route
				}(),
			},
			want: "duplicate operation ID",
		},
		{
			name: "missing path parameter",
			routes: []routes.Route{
				func() routes.Route {
					route := valid.Clone()
					route.Path = "/widgets/{widgetID}"
					return route
				}(),
			},
			want: "has no parameter metadata",
		},
		{
			name: "missing response",
			routes: []routes.Route{
				func() routes.Route {
					route := valid.Clone()
					route.Responses = nil
					return route
				}(),
			},
			want: "at least one response is required",
		},
		{
			name: "reserved header parameter",
			routes: []routes.Route{
				func() routes.Route {
					route := valid.Clone()
					route.Parameters = []routes.Parameter{
						{
							Name:   "Authorization",
							In:     routes.ParameterHeader,
							Schema: routes.SchemaOf[string](""),
						},
					}
					return route
				}(),
			},
			want: "must use response content, request body content, or a security scheme",
		},
		{
			name: "invalid media type",
			routes: []routes.Route{
				func() routes.Route {
					route := valid.Clone()
					route.Responses["200"] = routes.Response{
						Description: "Widgets",
						Content: routes.Content{
							"not-a-media-type": routes.SchemaOf[widgetResponse]("Widget"),
						},
					}
					return route
				}(),
			},
			want: "invalid media type",
		},
		{
			name: "component conflict",
			routes: []routes.Route{
				func() routes.Route {
					route := valid.Clone()
					route.Responses["200"] = routes.Response{
						Description: "Widget",
						Content: routes.Content{
							"application/json": routes.SchemaOf[widgetResponse]("Payload"),
						},
					}
					return route
				}(),
				{
					Method:      http.MethodPost,
					Path:        "/widgets",
					OperationID: "createWidget",
					Responses: map[string]routes.Response{
						"200": {
							Description: "Updated widget",
							Content: routes.Content{
								"application/json": routes.SchemaOf[updateWidgetRequest]("Payload"),
							},
						},
					},
				},
			},
			want: "schema component \"Payload\" refers to both",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := openapi.Generate(test.routes, openapi.Options{Title: "Test", Version: "1"})
			if err == nil {
				t.Fatal("expected contract error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
			if !strings.Contains(err.Error(), "route[") {
				t.Fatalf("expected route location, got %v", err)
			}
		})
	}
}
