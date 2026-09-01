package routes

import (
	"maps"
	"reflect"
	"slices"
	"strconv"
)

const jsonMediaType = "application/json"

// SchemaRef identifies the Go type and optional stable component name used to generate a schema.
// Unnamed references must not be used as stable component contracts.
// SchemaRef 标识用于生成 schema 的 Go 类型和可选 component 名。
// 未命名的引用不能作为稳定的 component 契约使用。
type SchemaRef struct {
	Name string
	Type reflect.Type
}

// SchemaOf returns a schema reference for T.
// Name must be stable once generated clients depend on it.
// SchemaOf 返回 T 的 schema 引用。
// 生成的 client 开始依赖 Name 后，Name 必须保持稳定。
func SchemaOf[T any](name string) SchemaRef {
	return SchemaRef{Name: name, Type: reflect.TypeFor[T]()}
}

// Content maps media types to schemas.
// Content 将媒体类型映射到 schema。
type Content map[string]SchemaRef

// ParameterLocation identifies where an operation parameter is sent.
// ParameterLocation 标识 operation 参数的传递位置。
type ParameterLocation string

const (
	// ParameterPath identifies a path parameter.
	// ParameterPath 表示 path 参数。
	ParameterPath ParameterLocation = "path"
	// ParameterQuery identifies a query parameter.
	// ParameterQuery 表示 query 参数。
	ParameterQuery ParameterLocation = "query"
	// ParameterHeader identifies a header parameter.
	// ParameterHeader 表示 header 参数。
	ParameterHeader ParameterLocation = "header"
	// ParameterCookie identifies a cookie parameter.
	// ParameterCookie 表示 cookie 参数。
	ParameterCookie ParameterLocation = "cookie"
)

// Parameter describes one operation parameter.
// Path parameters must be required.
// Parameter 描述一个 operation 参数。
// Path 参数必须为 required。
type Parameter struct {
	Name        string
	In          ParameterLocation
	Description string
	Required    bool
	Schema      SchemaRef
}

// RequestBody describes an operation request body.
// RequestBody 描述 operation 请求体。
type RequestBody struct {
	Description string
	Required    bool
	Content     Content
}

// Response describes one operation response.
// Empty Content represents a response without a body.
// Response 描述一个 operation 响应。
// Content 为空表示响应没有 body。
type Response struct {
	Description string
	Content     Content
}

// SecuritySchemeType identifies a supported security scheme shape.
// SecuritySchemeType 标识支持的 security scheme 形状。
type SecuritySchemeType string

const (
	// SecurityHTTP identifies an HTTP authentication scheme.
	// SecurityHTTP 表示 HTTP authentication scheme。
	SecurityHTTP SecuritySchemeType = "http"
	// SecurityAPIKey identifies an API key carried by a header, query, or cookie parameter.
	// SecurityAPIKey 表示通过 header、query 或 cookie 传递的 API key。
	SecurityAPIKey SecuritySchemeType = "apiKey"
)

// SecurityScheme defines a named security scheme used by a route.
// HTTP schemes use Scheme and BearerFormat. API-key schemes use ParameterName and In.
// SecurityScheme 定义路由使用的具名 security scheme。
// HTTP scheme 使用 Scheme 和 BearerFormat；API key scheme 使用 ParameterName 和 In。
type SecurityScheme struct {
	Name          string
	Type          SecuritySchemeType
	Scheme        string
	BearerFormat  string
	ParameterName string
	In            ParameterLocation
}

// SecurityRequirement lists schemes that must all succeed.
// Multiple route requirements are alternatives; an empty requirement allows anonymous access.
// SecurityRequirement 列出必须全部通过的 security scheme。
// 路由上的多个 requirement 互为替代；空 requirement 表示允许匿名访问。
type SecurityRequirement []SecurityScheme

// OperationID sets Route.OperationID.
// OperationID 设置 Route.OperationID。
func OperationID(id string) RouteOption {
	return func(r *Route) {
		r.OperationID = id
	}
}

// Parameters appends operation parameters.
// Parameters 追加 operation 参数。
func Parameters(params ...Parameter) RouteOption {
	return func(r *Route) {
		r.Parameters = append(r.Parameters, params...)
	}
}

// Body sets the request body.
// Body 设置请求体。
func Body(body RequestBody) RouteOption {
	return func(r *Route) {
		r.RequestBody = new(body.clone())
	}
}

// JSONBody sets an application/json request body.
// JSONBody 设置 application/json 请求体。
func JSONBody(schema SchemaRef, required bool) RouteOption {
	return Body(RequestBody{
		Required: required,
		Content:  Content{jsonMediaType: schema},
	})
}

// Respond sets the response selected by status.
// Status may be an HTTP status code, a range such as 4XX, or default.
// Respond 设置 status 对应的响应。
// Status 可以是 HTTP 状态码、4XX 形式的范围或 default。
func Respond(status string, response Response) RouteOption {
	return func(r *Route) {
		if r.Responses == nil {
			r.Responses = make(map[string]Response)
		}
		r.Responses[status] = response.clone()
	}
}

// JSONResponse sets an application/json response.
// JSONResponse 设置 application/json 响应。
func JSONResponse(status int, description string, schema SchemaRef) RouteOption {
	return Respond(strconv.Itoa(status), Response{
		Description: description,
		Content:     Content{jsonMediaType: schema},
	})
}

// EmptyResponse sets a response without a body.
// EmptyResponse 设置没有 body 的响应。
func EmptyResponse(status int, description string) RouteOption {
	return Respond(strconv.Itoa(status), Response{Description: description})
}

// Security appends alternative security requirements.
// Security 追加互为替代的 security requirement。
func Security(requirements ...SecurityRequirement) RouteOption {
	return func(r *Route) {
		r.Security = append(r.Security, cloneSecurity(requirements)...)
	}
}

func (b RequestBody) clone() RequestBody {
	b.Content = cloneContent(b.Content)
	return b
}

func (r Response) clone() Response {
	r.Content = cloneContent(r.Content)
	return r
}

func cloneContent(content Content) Content {
	return maps.Clone(content)
}

func cloneParameters(params []Parameter) []Parameter {
	return slices.Clone(params)
}

func cloneResponses(responses map[string]Response) map[string]Response {
	if responses == nil {
		return nil
	}
	out := make(map[string]Response, len(responses))
	for status, response := range responses {
		out[status] = response.clone()
	}
	return out
}

func cloneSecurity(requirements []SecurityRequirement) []SecurityRequirement {
	if requirements == nil {
		return nil
	}
	out := make([]SecurityRequirement, len(requirements))
	for i, requirement := range requirements {
		out[i] = slices.Clone(requirement)
	}
	return out
}
