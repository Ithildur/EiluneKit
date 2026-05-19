package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const defaultOpenAPISecurityScheme = "BearerAuth"

// OpenAPIOptions configures ExportOpenAPI.
// OpenAPIOptions 配置 ExportOpenAPI。
type OpenAPIOptions struct {
	// Title is the OpenAPI info title.
	// Title 是 OpenAPI info title。
	Title string
	// Version is the OpenAPI info version.
	// Version 是 OpenAPI info version。
	Version string
	// Description is the optional OpenAPI info description.
	// Description 是可选的 OpenAPI info description。
	Description string
	// BearerSecurityScheme is the bearer auth security scheme name.
	// BearerSecurityScheme 是 bearer auth security scheme 名。
	BearerSecurityScheme string
}

type openAPIDocument struct {
	OpenAPI    string                 `json:"openapi"`
	Info       openAPIInfo            `json:"info"`
	Paths      map[string]openAPIPath `json:"paths"`
	Components *openAPIComponents     `json:"components,omitempty"`
}

type openAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

type openAPIPath map[string]openAPIOperation

type openAPIOperation struct {
	Summary   string                 `json:"summary,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Security  []map[string][]string  `json:"security,omitempty"`
	Responses map[string]openAPIResp `json:"responses"`
}

type openAPIResp struct {
	Description string `json:"description"`
}

type openAPIComponents struct {
	SecuritySchemes map[string]openAPISecurityScheme `json:"securitySchemes,omitempty"`
}

type openAPISecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme"`
	BearerFormat string `json:"bearerFormat,omitempty"`
}

// ExportOpenAPI returns OpenAPI 3.0 JSON from route metadata.
// ExportOpenAPI 根据路由元数据返回 OpenAPI 3.0 JSON。
func ExportOpenAPI(routes []Route, opts OpenAPIOptions) ([]byte, error) {
	doc, err := buildOpenAPI(routes, opts)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(doc, "", "  ")
}

func buildOpenAPI(routes []Route, opts OpenAPIOptions) (openAPIDocument, error) {
	opts = normalizeOpenAPIOptions(opts)
	doc := openAPIDocument{
		OpenAPI: "3.0.3",
		Info: openAPIInfo{
			Title:       opts.Title,
			Version:     opts.Version,
			Description: opts.Description,
		},
		Paths: make(map[string]openAPIPath, len(routes)),
	}

	usesBearer := false
	for i, route := range routes {
		method, path, err := normalizeRoute(route.Method, route.Path)
		if err != nil {
			return openAPIDocument{}, fmt.Errorf("openapi: route[%d]: %w", i, err)
		}
		method, err = openAPIMethod(method)
		if err != nil {
			return openAPIDocument{}, fmt.Errorf("openapi: route[%d] %s %s: %w", i, route.Method, route.Path, err)
		}
		if _, ok := doc.Paths[path]; !ok {
			doc.Paths[path] = make(openAPIPath)
		}
		if _, ok := doc.Paths[path][method]; ok {
			return openAPIDocument{}, fmt.Errorf("openapi: duplicate operation: %s %s", strings.ToUpper(method), path)
		}

		op := openAPIOperation{
			Summary: route.Summary,
			Tags:    append([]string(nil), route.Tags...),
			Responses: map[string]openAPIResp{
				"default": {Description: "Default response"},
			},
		}
		switch effectiveAuth(route.Auth) {
		case AuthRequired:
			usesBearer = true
			op.Security = []map[string][]string{{opts.BearerSecurityScheme: {}}}
		case AuthOptional:
			usesBearer = true
			op.Security = []map[string][]string{{opts.BearerSecurityScheme: {}}, {}}
		}
		doc.Paths[path][method] = op
	}

	if usesBearer {
		doc.Components = &openAPIComponents{
			SecuritySchemes: map[string]openAPISecurityScheme{
				opts.BearerSecurityScheme: {
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "JWT",
				},
			},
		}
	}
	return doc, nil
}

func normalizeOpenAPIOptions(opts OpenAPIOptions) OpenAPIOptions {
	opts.Title = strings.TrimSpace(opts.Title)
	if opts.Title == "" {
		opts.Title = "API"
	}
	opts.Version = strings.TrimSpace(opts.Version)
	if opts.Version == "" {
		opts.Version = "0.0.0"
	}
	opts.Description = strings.TrimSpace(opts.Description)
	opts.BearerSecurityScheme = strings.TrimSpace(opts.BearerSecurityScheme)
	if opts.BearerSecurityScheme == "" {
		opts.BearerSecurityScheme = defaultOpenAPISecurityScheme
	}
	return opts
}

func openAPIMethod(method string) (string, error) {
	switch method {
	case http.MethodGet:
		return "get", nil
	case http.MethodPost:
		return "post", nil
	case http.MethodPut:
		return "put", nil
	case http.MethodPatch:
		return "patch", nil
	case http.MethodDelete:
		return "delete", nil
	case http.MethodOptions:
		return "options", nil
	case http.MethodHead:
		return "head", nil
	case http.MethodTrace:
		return "trace", nil
	default:
		return "", fmt.Errorf("unsupported method %q", method)
	}
}
