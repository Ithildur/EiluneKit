// Package passkeyhttp shares the WebAuthn transport between auth adapters.
// Package passkeyhttp 在 auth 适配器之间共享 WebAuthn 传输处理。
package passkeyhttp

import (
	"encoding/json/jsontext"
	"errors"
	"net/http"

	"github.com/Ithildur/EiluneKit/auth/passkey"
	"github.com/Ithildur/EiluneKit/http/decoder"
	"github.com/Ithildur/EiluneKit/http/middleware"
	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"
	"github.com/go-chi/chi/v5"
	"github.com/invopop/jsonschema"
)

// Handler shares ceremony and management routes; adapters own session issuance.
// Handler 共享流程与管理路由；适配器负责会话签发。
type Handler struct {
	Service      *passkey.Service
	Authenticate routes.Middleware
	UserID       func(*http.Request) (string, bool)
	Login        func(http.ResponseWriter, *http.Request, string, []byte)
	Failure      func(http.ResponseWriter, error)
}

type request struct {
	Operation   string         `json:"operation" jsonschema:"enum=register.begin,enum=register.finish,enum=login.begin,enum=login.finish"`
	FlowID      string         `json:"flow_id,omitempty"`
	Name        string         `json:"name,omitempty"`
	Persistence string         `json:"persistence,omitempty" jsonschema:"enum=session,enum=persistent"`
	Credential  jsontext.Value `json:"credential,omitempty"`
}

func (request) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.Properties.Set("credential", &jsonschema.Schema{Type: "object"})
}

type renameRequest struct {
	Name string `json:"name"`
}
type listResponse struct {
	Credentials []passkey.Info `json:"credentials"`
}

// Routes builds relative routes with body limits and the adapter's rate limiter.
// Routes 构造相对路由，使用请求体限制及适配器的限流器。
func (h Handler) Routes(maxBody int64, rate routes.Middleware) []routes.Route {
	b := routes.NewBlueprint(routes.DefaultAuth(routes.AuthPublic))
	opts := []routes.RouteOption{
		routes.OperationID("PasskeyCeremony"),
		routes.Auth(routes.AuthOptional),
		routes.Security(routes.SecurityRequirement{}, routes.SecurityRequirement{{Name: "BearerAuth", Type: routes.SecurityHTTP, Scheme: "bearer"}}),
		routes.JSONBody(routes.SchemaOf[request]("PasskeyRequest"), true),
		routes.JSONResponse(http.StatusOK, "Begin: flow_id and options; register.finish: credential metadata; login.finish: the auth adapter's login response", routes.SchemaOf[any]("")),
		routes.Use(middleware.LimitBody(maxBody), middleware.RequireJSONBody),
	}
	if rate != nil {
		opts = append(opts, routes.Use(rate))
	}
	for _, status := range []int{400, 401, 409, 413, 415, 429, 500, 503} {
		opts = append(opts, routes.JSONResponse(status, http.StatusText(status), routes.SchemaOf[response.ErrorResponse]("ErrorResponse")))
	}
	b.Post("/passkey", "Register or authenticate a passkey; registration requires a user bearer token", h.ceremony, opts...)
	management := routes.NewBlueprint(routes.DefaultAuth(routes.AuthRequired), routes.DefaultMiddleware(h.Authenticate))
	security := routes.Security(routes.SecurityRequirement{{Name: "BearerAuth", Type: routes.SecurityHTTP, Scheme: "bearer"}})
	common := []routes.RouteOption{security}
	for _, status := range []int{400, 401, 404, 413, 415, 500, 503} {
		common = append(common, routes.JSONResponse(status, http.StatusText(status), routes.SchemaOf[response.ErrorResponse]("ErrorResponse")))
	}
	management.Get("", "List passkeys", h.list, appendCopy(common,
		routes.OperationID("ListPasskeys"), routes.JSONResponse(200, "Credentials", routes.SchemaOf[listResponse]("PasskeyListResponse")))...)
	item := []routes.RouteOption{routes.Parameters(routes.Parameter{Name: "id", In: routes.ParameterPath, Required: true, Schema: routes.SchemaOf[string]("")})}
	management.Patch("/{id}", "Rename passkey", h.rename, appendCopy(common, append(item,
		routes.OperationID("RenamePasskey"), routes.JSONBody(routes.SchemaOf[renameRequest]("PasskeyRenameRequest"), true),
		routes.Use(middleware.LimitBody(maxBody), middleware.RequireJSONBody), routes.EmptyResponse(204, "Renamed"))...)...)
	management.Delete("/{id}", "Delete passkey", h.delete, appendCopy(common, append(item,
		routes.OperationID("DeletePasskey"), routes.EmptyResponse(204, "Deleted"))...)...)
	b.Include("/passkeys", management)
	return b.Routes()
}

func appendCopy(base []routes.RouteOption, more ...routes.RouteOption) []routes.RouteOption {
	return append(append([]routes.RouteOption(nil), base...), more...)
}

func (h Handler) ceremony(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req request
	if !h.decode(w, r, &req) {
		return
	}
	switch req.Operation {
	case "register.begin", "register.finish":
		h.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := h.user(w, r)
			if !ok {
				return
			}
			if req.Operation == "register.begin" {
				result, err := h.Service.BeginRegistration(r.Context(), id)
				if err != nil {
					h.fail(w, err)
					return
				}
				response.WriteJSON(w, 200, result)
				return
			}
			result, err := h.Service.FinishRegistration(r.Context(), id, req.FlowID, req.Name, req.Credential)
			if err != nil {
				h.fail(w, err)
				return
			}
			response.WriteJSON(w, 200, result)
		})).ServeHTTP(w, r)
	case "login.begin":
		if req.Persistence != "session" && req.Persistence != "persistent" {
			response.WriteJSONError(w, 400, "invalid_persistence", "persistence must be session or persistent")
			return
		}
		result, err := h.Service.BeginLogin(r.Context(), req.Persistence == "session")
		if err != nil {
			h.fail(w, err)
			return
		}
		response.WriteJSON(w, 200, result)
	case "login.finish":
		h.Login(w, r, req.FlowID, req.Credential)
	default:
		response.WriteJSONError(w, 400, "invalid_operation", "unknown passkey operation")
	}
}

func (h Handler) user(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := h.UserID(r)
	if !ok {
		response.WriteJSONError(w, 401, "unauthorized", "user authentication required")
	}
	return id, ok
}

func (h Handler) list(w http.ResponseWriter, r *http.Request) {
	id, ok := h.user(w, r)
	if !ok {
		return
	}
	items, err := h.Service.List(r.Context(), id)
	if err != nil {
		h.fail(w, err)
		return
	}
	response.WriteJSON(w, 200, listResponse{Credentials: items})
}

func (h Handler) rename(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	id, ok := h.user(w, r)
	if !ok {
		return
	}
	var req renameRequest
	if !h.decode(w, r, &req) {
		return
	}
	if err := h.Service.Rename(r.Context(), id, chi.URLParam(r, "id"), req.Name); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(204)
}

func (h Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.user(w, r)
	if !ok {
		return
	}
	if err := h.Service.Delete(r.Context(), id, chi.URLParam(r, "id")); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(204)
}

func (h Handler) decode(w http.ResponseWriter, r *http.Request, out any) bool {
	if err := decoder.DecodeJSONBodyWithOptions(r, out, decoder.JSONOptions{DisallowUnknownFields: true}); err != nil {
		if errors.Is(err, decoder.ErrBodyTooLarge) {
			response.WriteJSONError(w, 413, "body_too_large", "request body too large")
		} else {
			response.WriteJSONError(w, 400, "invalid_json", "invalid json")
		}
		return false
	}
	return true
}

func (h Handler) fail(w http.ResponseWriter, err error) {
	if !WriteError(w, err) {
		h.Failure(w, err)
	}
}

// WriteError writes known passkey errors; false leaves adapter errors to the caller.
// WriteError 写出已知通行密钥错误；false 表示由调用方处理适配器错误。
func WriteError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, passkey.ErrRejected):
		response.WriteJSONError(w, 401, "unauthorized", "invalid credentials")
	case errors.Is(err, passkey.ErrNotFound):
		response.WriteJSONError(w, 404, "passkey_not_found", "passkey not found")
	case errors.Is(err, passkey.ErrConflict):
		response.WriteJSONError(w, 409, "passkey_conflict", "passkey already exists")
	case errors.Is(err, passkey.ErrInvalidName):
		response.WriteJSONError(w, 400, "invalid_name", passkey.ErrInvalidName.Error())
	case errors.Is(err, passkey.ErrCapacity):
		response.WriteJSONError(w, 503, "passkey_unavailable", "passkey flow capacity reached")
	default:
		return false
	}
	return true
}
