package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"mime"
	"net/http"
	pathpkg "path"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/Ithildur/EiluneKit/http/routes"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/invopop/jsonschema"
)

const version = "3.1.1"

var responseStatusPattern = regexp.MustCompile(`^(?:[1-5][0-9]{2}|[1-5]XX|default)$`)

// Options configures OpenAPI document generation.
// Title and Version are required.
// Options 配置 OpenAPI 文档生成。
// Title 和 Version 必填。
type Options struct {
	Title       string
	Version     string
	Description string
}

// Generate returns validated OpenAPI 3.1 JSON for routeList.
// Route paths must include their final mount prefixes.
// Generate 返回 routeList 对应且已通过校验的 OpenAPI 3.1 JSON。
// 路由 path 必须包含最终挂载前缀。
func Generate(routeList []routes.Route, opts Options) ([]byte, error) {
	opts.Title = strings.TrimSpace(opts.Title)
	if opts.Title == "" {
		return nil, fmt.Errorf("openapi: title is required")
	}
	opts.Version = strings.TrimSpace(opts.Version)
	if opts.Version == "" {
		return nil, fmt.Errorf("openapi: version is required")
	}
	opts.Description = strings.TrimSpace(opts.Description)

	b := newBuilder(routeList)
	if err := b.collectSchemaNames(); err != nil {
		return nil, err
	}
	doc, err := b.build(opts)
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("openapi: encode document: %w", err)
	}

	loader := openapi3.NewLoader()
	loaded, err := loader.LoadFromData(payload)
	if err != nil {
		return nil, fmt.Errorf("openapi: load generated document: %w", err)
	}
	if err := loaded.Validate(context.Background(), openapi3.EnableMultiError()); err != nil {
		return nil, fmt.Errorf("openapi: validate generated document: %w", err)
	}
	return payload, nil
}

type builder struct {
	routes             []routes.Route
	doc                *openapi3.T
	componentNames     map[reflect.Type]string
	componentTypes     map[string]reflect.Type
	schemaCache        map[schemaKey]*openapi3.SchemaRef
	componentJSON      map[string]string
	securitySchemes    map[string]routes.SecurityScheme
	operationIDs       map[string]struct{}
	normalizedRouteKey map[string]struct{}
	reflector          *jsonschema.Reflector
}

type schemaKey struct {
	name   string
	typeOf reflect.Type
}

func newBuilder(routeList []routes.Route) *builder {
	b := &builder{
		routes:             routeList,
		componentNames:     make(map[reflect.Type]string),
		componentTypes:     make(map[string]reflect.Type),
		schemaCache:        make(map[schemaKey]*openapi3.SchemaRef),
		componentJSON:      make(map[string]string),
		securitySchemes:    make(map[string]routes.SecurityScheme),
		operationIDs:       make(map[string]struct{}),
		normalizedRouteKey: make(map[string]struct{}),
	}
	b.reflector = &jsonschema.Reflector{
		Anonymous: true,
		Namer:     b.schemaName,
	}
	return b
}

func (b *builder) build(opts Options) (*openapi3.T, error) {
	b.doc = &openapi3.T{
		OpenAPI: version,
		Info: &openapi3.Info{
			Title:       opts.Title,
			Version:     opts.Version,
			Description: opts.Description,
		},
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas:         make(openapi3.Schemas),
			SecuritySchemes: make(openapi3.SecuritySchemes),
		},
		JSONSchemaDialect: jsonschema.Version,
	}

	for i, route := range b.routes {
		if err := b.addRoute(i, route); err != nil {
			return nil, err
		}
	}
	if len(b.doc.Components.Schemas) == 0 && len(b.doc.Components.SecuritySchemes) == 0 {
		b.doc.Components = nil
	}
	return b.doc, nil
}

func (b *builder) addRoute(index int, route routes.Route) error {
	method, err := normalizeMethod(route.Method)
	if err != nil {
		return routeError(index, route, err)
	}
	path, pathParams, err := normalizePath(route.Path)
	if err != nil {
		return routeError(index, route, err)
	}
	operationID := strings.TrimSpace(route.OperationID)
	if operationID == "" {
		return routeError(index, route, fmt.Errorf("operation ID is required"))
	}
	if operationID != route.OperationID {
		return routeError(index, route, fmt.Errorf("operation ID must not contain surrounding whitespace"))
	}
	if _, exists := b.operationIDs[operationID]; exists {
		return routeError(index, route, fmt.Errorf("duplicate operation ID %q", operationID))
	}
	b.operationIDs[operationID] = struct{}{}

	routeKey := method + " " + path
	if _, exists := b.normalizedRouteKey[routeKey]; exists {
		return routeError(index, route, fmt.Errorf("duplicate operation after path normalization: %s", routeKey))
	}
	b.normalizedRouteKey[routeKey] = struct{}{}

	security, err := b.buildSecurity(index, route)
	if err != nil {
		return err
	}

	params, err := b.buildParameters(index, route, pathParams)
	if err != nil {
		return err
	}
	body, err := b.buildRequestBody(index, route)
	if err != nil {
		return err
	}
	responses, err := b.buildResponses(index, route)
	if err != nil {
		return err
	}

	operation := &openapi3.Operation{
		Tags:        slices.Clone(route.Tags),
		Summary:     route.Summary,
		OperationID: operationID,
		Parameters:  params,
		RequestBody: body,
		Responses:   responses,
	}
	if route.Security != nil {
		operation.Security = &security
	}

	pathItem := b.doc.Paths.Value(path)
	if pathItem == nil {
		pathItem = &openapi3.PathItem{}
		b.doc.Paths.Set(path, pathItem)
	}
	pathItem.SetOperation(method, operation)
	return nil
}

func (b *builder) buildParameters(index int, route routes.Route, pathNames map[string]struct{}) (openapi3.Parameters, error) {
	params := make(openapi3.Parameters, 0, len(route.Parameters))
	seen := make(map[string]struct{}, len(route.Parameters))
	pathParams := make(map[string]struct{}, len(pathNames))

	for _, param := range route.Parameters {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			return nil, routeError(index, route, fmt.Errorf("parameter name is required"))
		}
		if name != param.Name {
			return nil, routeError(index, route, fmt.Errorf("parameter %q contains surrounding whitespace", param.Name))
		}
		in := string(param.In)
		switch param.In {
		case routes.ParameterPath, routes.ParameterQuery, routes.ParameterHeader, routes.ParameterCookie:
		default:
			return nil, routeError(index, route, fmt.Errorf("parameter %q has unsupported location %q", name, param.In))
		}
		keyName := name
		if param.In == routes.ParameterHeader {
			keyName = strings.ToLower(name)
			switch keyName {
			case "accept", "content-type", "authorization":
				return nil, routeError(index, route, fmt.Errorf("header parameter %q must use response content, request body content, or a security scheme", name))
			}
		}
		key := in + ":" + keyName
		if _, exists := seen[key]; exists {
			return nil, routeError(index, route, fmt.Errorf("duplicate parameter %s", key))
		}
		seen[key] = struct{}{}
		if param.In == routes.ParameterPath {
			if !param.Required {
				return nil, routeError(index, route, fmt.Errorf("path parameter %q must be required", name))
			}
			if _, exists := pathNames[name]; !exists {
				return nil, routeError(index, route, fmt.Errorf("path parameter %q is not present in the path", name))
			}
			pathParams[name] = struct{}{}
		}

		schema, err := b.schema(param.Schema)
		if err != nil {
			return nil, routeError(index, route, fmt.Errorf("parameter %q schema: %w", name, err))
		}
		params = append(params, &openapi3.ParameterRef{Value: &openapi3.Parameter{
			Name:        name,
			In:          in,
			Description: param.Description,
			Required:    param.Required,
			Schema:      schema,
		}})
	}

	for name := range pathNames {
		if _, exists := pathParams[name]; !exists {
			return nil, routeError(index, route, fmt.Errorf("path parameter %q has no parameter metadata", name))
		}
	}
	return params, nil
}

func (b *builder) buildRequestBody(index int, route routes.Route) (*openapi3.RequestBodyRef, error) {
	if route.RequestBody == nil {
		return nil, nil
	}
	if len(route.RequestBody.Content) == 0 {
		return nil, routeError(index, route, fmt.Errorf("request body content is required"))
	}
	content, err := b.buildContent(route.RequestBody.Content)
	if err != nil {
		return nil, routeError(index, route, fmt.Errorf("request body: %w", err))
	}
	return &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{
		Description: route.RequestBody.Description,
		Required:    route.RequestBody.Required,
		Content:     content,
	}}, nil
}

func (b *builder) buildResponses(index int, route routes.Route) (*openapi3.Responses, error) {
	if len(route.Responses) == 0 {
		return nil, routeError(index, route, fmt.Errorf("at least one response is required"))
	}
	responses := openapi3.NewResponses()
	for _, status := range slices.Sorted(maps.Keys(route.Responses)) {
		if !responseStatusPattern.MatchString(status) {
			return nil, routeError(index, route, fmt.Errorf("invalid response status %q", status))
		}
		response := route.Responses[status]
		if strings.TrimSpace(response.Description) == "" {
			return nil, routeError(index, route, fmt.Errorf("response %s description is required", status))
		}
		content, err := b.buildContent(response.Content)
		if err != nil {
			return nil, routeError(index, route, fmt.Errorf("response %s: %w", status, err))
		}
		responses.Set(status, &openapi3.ResponseRef{Value: &openapi3.Response{
			Description: &response.Description,
			Content:     content,
		}})
	}
	return responses, nil
}

func (b *builder) buildContent(content routes.Content) (openapi3.Content, error) {
	if len(content) == 0 {
		return nil, nil
	}
	out := make(openapi3.Content, len(content))
	for _, mediaType := range slices.Sorted(maps.Keys(content)) {
		if err := validateMediaType(mediaType); err != nil {
			return nil, err
		}
		schema, err := b.schema(content[mediaType])
		if err != nil {
			return nil, fmt.Errorf("media type %q schema: %w", mediaType, err)
		}
		out[mediaType] = openapi3.NewMediaType().WithSchemaRef(schema)
	}
	return out, nil
}

func validateMediaType(value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("invalid media type %q", value)
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return fmt.Errorf("invalid media type %q: %w", value, err)
	}
	if _, subtype, ok := strings.Cut(mediaType, "/"); !ok || subtype == "" {
		return fmt.Errorf("invalid media type %q: type and subtype are required", value)
	}
	return nil
}

func (b *builder) addSecurityScheme(index int, route routes.Route, scheme routes.SecurityScheme) error {
	if err := openapi3.ValidateIdentifier(scheme.Name); err != nil {
		return routeError(index, route, fmt.Errorf("invalid security scheme name %q: %w", scheme.Name, err))
	}
	if err := validateSecurityScheme(scheme); err != nil {
		return routeError(index, route, fmt.Errorf("security scheme %q: %w", scheme.Name, err))
	}
	if existing, exists := b.securitySchemes[scheme.Name]; exists {
		if existing != scheme {
			return routeError(index, route, fmt.Errorf("security scheme %q has conflicting definitions", scheme.Name))
		}
		return nil
	}
	value := &openapi3.SecurityScheme{
		Type:         string(scheme.Type),
		Scheme:       scheme.Scheme,
		BearerFormat: scheme.BearerFormat,
		Name:         scheme.ParameterName,
		In:           string(scheme.In),
	}
	b.securitySchemes[scheme.Name] = scheme
	b.doc.Components.SecuritySchemes[scheme.Name] = &openapi3.SecuritySchemeRef{Value: value}
	return nil
}

func validateSecurityScheme(scheme routes.SecurityScheme) error {
	switch scheme.Type {
	case routes.SecurityHTTP:
		if strings.TrimSpace(scheme.Scheme) == "" {
			return fmt.Errorf("HTTP scheme is required")
		}
		if strings.TrimSpace(scheme.Scheme) != scheme.Scheme {
			return fmt.Errorf("HTTP scheme contains surrounding whitespace")
		}
		if strings.TrimSpace(scheme.BearerFormat) != scheme.BearerFormat {
			return fmt.Errorf("bearer format contains surrounding whitespace")
		}
		if scheme.BearerFormat != "" && !strings.EqualFold(scheme.Scheme, "bearer") {
			return fmt.Errorf("bearer format requires the bearer HTTP scheme")
		}
		if scheme.ParameterName != "" || scheme.In != "" {
			return fmt.Errorf("HTTP scheme must not declare an API-key parameter")
		}
	case routes.SecurityAPIKey:
		if strings.TrimSpace(scheme.ParameterName) == "" {
			return fmt.Errorf("API-key parameter name is required")
		}
		if strings.TrimSpace(scheme.ParameterName) != scheme.ParameterName {
			return fmt.Errorf("API-key parameter name contains surrounding whitespace")
		}
		switch scheme.In {
		case routes.ParameterHeader, routes.ParameterQuery, routes.ParameterCookie:
		default:
			return fmt.Errorf("API-key location must be header, query, or cookie")
		}
		if scheme.Scheme != "" || scheme.BearerFormat != "" {
			return fmt.Errorf("API-key scheme must not declare HTTP scheme fields")
		}
	default:
		return fmt.Errorf("unsupported type %q", scheme.Type)
	}
	return nil
}

func (b *builder) buildSecurity(index int, route routes.Route) (openapi3.SecurityRequirements, error) {
	requirements := make(openapi3.SecurityRequirements, len(route.Security))
	hasEmpty := false
	hasSecured := false
	for i, requirement := range route.Security {
		converted := make(openapi3.SecurityRequirement, len(requirement))
		if len(requirement) == 0 {
			hasEmpty = true
		} else {
			hasSecured = true
		}
		for _, scheme := range requirement {
			if err := b.addSecurityScheme(index, route, scheme); err != nil {
				return nil, err
			}
			if _, exists := converted[scheme.Name]; exists {
				return nil, routeError(index, route, fmt.Errorf("duplicate security scheme %q in one requirement", scheme.Name))
			}
			converted[scheme.Name] = []string{}
		}
		requirements[i] = converted
	}

	auth := route.Auth
	if auth == "" {
		auth = routes.AuthPublic
	}
	switch auth {
	case routes.AuthPublic:
		if hasSecured {
			return nil, routeError(index, route, fmt.Errorf("public route must not require a security scheme"))
		}
	case routes.AuthRequired:
		if !hasSecured || hasEmpty {
			return nil, routeError(index, route, fmt.Errorf("required-auth route must declare only secured alternatives"))
		}
	case routes.AuthOptional:
		if !hasSecured || !hasEmpty {
			return nil, routeError(index, route, fmt.Errorf("optional-auth route must declare secured and anonymous alternatives"))
		}
	default:
		return nil, routeError(index, route, fmt.Errorf("unsupported auth requirement %q", route.Auth))
	}
	return requirements, nil
}

func normalizeMethod(raw string) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(raw))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
		http.MethodOptions, http.MethodHead, http.MethodTrace:
		return method, nil
	case "":
		return "", fmt.Errorf("HTTP method is required")
	default:
		return "", fmt.Errorf("unsupported HTTP method %q", raw)
	}
}

func normalizePath(raw string) (string, map[string]struct{}, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		path = "/"
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	params := make(map[string]struct{})
	var out strings.Builder
	out.Grow(len(path))
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '*':
			return "", nil, fmt.Errorf("wildcard paths cannot be represented in OpenAPI 3.1")
		case '}':
			return "", nil, fmt.Errorf("unmatched closing brace in path %q", path)
		case '{':
			end := pathParamEnd(path, i)
			if end < 0 {
				return "", nil, fmt.Errorf("unclosed path parameter in %q", path)
			}
			rawParam := path[i+1 : end]
			name, _, _ := strings.Cut(rawParam, ":")
			trimmedName := strings.TrimSpace(name)
			if name != trimmedName {
				return "", nil, fmt.Errorf("path parameter %q contains surrounding whitespace", name)
			}
			name = trimmedName
			if name == "" {
				return "", nil, fmt.Errorf("empty path parameter in %q", path)
			}
			if _, exists := params[name]; exists {
				return "", nil, fmt.Errorf("duplicate path parameter %q", name)
			}
			params[name] = struct{}{}
			out.WriteByte('{')
			out.WriteString(name)
			out.WriteByte('}')
			i = end
		default:
			out.WriteByte(path[i])
		}
	}
	return out.String(), params, nil
}

func pathParamEnd(path string, start int) int {
	depth := 0
	for i := start; i < len(path); i++ {
		switch path[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func routeError(index int, route routes.Route, err error) error {
	method := strings.ToUpper(strings.TrimSpace(route.Method))
	path := strings.TrimSpace(route.Path)
	return fmt.Errorf("openapi: route[%d] %s %s operationId=%q: %w", index, method, path, route.OperationID, err)
}

func (b *builder) collectSchemaNames() error {
	for i, route := range b.routes {
		for _, schema := range routeSchemas(route) {
			if schema.Type == nil {
				return routeError(i, route, fmt.Errorf("schema type is required"))
			}
			if schema.Name == "" {
				continue
			}
			if strings.TrimSpace(schema.Name) != schema.Name {
				return routeError(i, route, fmt.Errorf("schema name %q contains surrounding whitespace", schema.Name))
			}
			if err := openapi3.ValidateIdentifier(schema.Name); err != nil {
				return routeError(i, route, fmt.Errorf("invalid schema name %q: %w", schema.Name, err))
			}
			typeOf := indirectType(schema.Type)
			if existing, ok := b.componentTypes[schema.Name]; ok && existing != typeOf {
				return routeError(i, route, fmt.Errorf("schema component %q refers to both %s and %s", schema.Name, existing, typeOf))
			}
			if existing, ok := b.componentNames[typeOf]; ok && existing != schema.Name {
				return routeError(i, route, fmt.Errorf("Go type %s uses both schema names %q and %q", typeOf, existing, schema.Name))
			}
			b.componentTypes[schema.Name] = typeOf
			b.componentNames[typeOf] = schema.Name
		}
	}
	return nil
}

func routeSchemas(route routes.Route) []routes.SchemaRef {
	count := len(route.Parameters)
	if route.RequestBody != nil {
		count += len(route.RequestBody.Content)
	}
	for _, response := range route.Responses {
		count += len(response.Content)
	}
	out := make([]routes.SchemaRef, 0, count)
	for _, param := range route.Parameters {
		out = append(out, param.Schema)
	}
	if route.RequestBody != nil {
		for _, mediaType := range slices.Sorted(maps.Keys(route.RequestBody.Content)) {
			out = append(out, route.RequestBody.Content[mediaType])
		}
	}
	for _, status := range slices.Sorted(maps.Keys(route.Responses)) {
		response := route.Responses[status]
		for _, mediaType := range slices.Sorted(maps.Keys(response.Content)) {
			out = append(out, response.Content[mediaType])
		}
	}
	return out
}

func (b *builder) schema(ref routes.SchemaRef) (*openapi3.SchemaRef, error) {
	if ref.Type == nil {
		return nil, fmt.Errorf("schema type is required")
	}
	key := schemaKey{name: ref.Name, typeOf: ref.Type}
	if cached := b.schemaCache[key]; cached != nil {
		return cached, nil
	}

	reflected, err := b.reflectSchema(ref.Type)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(reflected)
	if err != nil {
		return nil, fmt.Errorf("encode reflected schema for %s: %w", ref.Type, err)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode reflected schema for %s: %w", ref.Type, err)
	}
	rewriteRefs(raw)

	if object, ok := raw.(map[string]any); ok {
		if defs, ok := object["$defs"].(map[string]any); ok {
			for _, name := range slices.Sorted(maps.Keys(defs)) {
				component, err := decodeSchemaRef(defs[name])
				if err != nil {
					return nil, fmt.Errorf("decode schema component %q: %w", name, err)
				}
				if err := b.addSchemaComponent(name, component); err != nil {
					return nil, err
				}
			}
		}
		delete(object, "$defs")
		delete(object, "$schema")
		delete(object, "$id")
	}

	schema, err := decodeSchemaRef(raw)
	if err != nil {
		return nil, fmt.Errorf("decode schema for %s: %w", ref.Type, err)
	}
	if ref.Name != "" {
		target := "#/components/schemas/" + ref.Name
		if schema.Ref != target {
			if err := b.addSchemaComponent(ref.Name, schema); err != nil {
				return nil, err
			}
			schema = &openapi3.SchemaRef{Ref: target}
		}
	}
	b.schemaCache[key] = schema
	return schema, nil
}

func (b *builder) reflectSchema(typeOf reflect.Type) (schema *jsonschema.Schema, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("reflect Go type %s: %v", typeOf, recovered)
		}
	}()
	return b.reflector.ReflectFromType(typeOf), nil
}

func (b *builder) addSchemaComponent(name string, schema *openapi3.SchemaRef) error {
	if err := openapi3.ValidateIdentifier(name); err != nil {
		return fmt.Errorf("invalid generated schema component name %q: %w", name, err)
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("encode schema component %q: %w", name, err)
	}
	encoded := string(data)
	if existing, ok := b.componentJSON[name]; ok {
		if existing != encoded {
			return fmt.Errorf("schema component %q has conflicting definitions", name)
		}
		return nil
	}
	b.componentJSON[name] = encoded
	b.doc.Components.Schemas[name] = schema
	return nil
}

func (b *builder) schemaName(typeOf reflect.Type) string {
	typeOf = indirectType(typeOf)
	if name := b.componentNames[typeOf]; name != "" {
		return name
	}
	name := typeOf.Name()
	if name == "" {
		return ""
	}
	if pkg := pathpkg.Base(typeOf.PkgPath()); pkg != "." && pkg != "" {
		name = pkg + "." + name
	}
	return sanitizeComponentName(name)
}

func indirectType(typeOf reflect.Type) reflect.Type {
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	return typeOf
}

func sanitizeComponentName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, name)
}

func rewriteRefs(node any) {
	switch value := node.(type) {
	case map[string]any:
		if ref, ok := value["$ref"].(string); ok {
			if name, found := strings.CutPrefix(ref, "#/$defs/"); found {
				value["$ref"] = "#/components/schemas/" + name
			}
		}
		for _, child := range value {
			rewriteRefs(child)
		}
	case []any:
		for _, child := range value {
			rewriteRefs(child)
		}
	}
}

func decodeSchemaRef(raw any) (*openapi3.SchemaRef, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var schema openapi3.SchemaRef
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}
