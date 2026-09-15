package response_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/Ithildur/EiluneKit/http/middleware"
	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"
)

func ExampleNotFound() {
	api := routes.NewBlueprint()
	endpoint := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
	api.Get("/items", "", endpoint)
	api.Get("/private", "", endpoint, routes.Auth(routes.AuthRequired))
	api.Get("/panic", "", func(http.ResponseWriter, *http.Request) { panic("private details") })
	h, err := routes.NewHandler(api.Routes(), routes.HandlerOptions{
		NotFound:         response.NotFound(),
		MethodNotAllowed: response.MethodNotAllowed(),
		Unauthorized:     response.Unauthorized(),
		Middleware: []routes.Middleware{
			middleware.Recover(middleware.RecoverOptions{
				Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
				OnPanic: response.InternalServerError(),
			}),
		},
	})
	if err != nil {
		panic(err)
	}
	for _, request := range []struct{ method, path string }{
		{"GET", "/missing"}, {"POST", "/items"}, {"GET", "/private"}, {"GET", "/panic"},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(request.method, request.path, nil))
		fmt.Println(w.Code, w.Header().Get("Content-Type"), w.Body.String())
		if w.Code == http.StatusMethodNotAllowed {
			fmt.Println("Allow:", w.Header().Get("Allow"))
		}
	}
	// Output:
	// 404 application/json; charset=utf-8 {"code":"not_found","message":"resource not found"}
	// 405 application/json; charset=utf-8 {"code":"method_not_allowed","message":"method not allowed"}
	// Allow: GET
	// 401 application/json; charset=utf-8 {"code":"unauthorized","message":"authentication required"}
	// 500 application/json; charset=utf-8 {"code":"internal_error","message":"internal server error"}
}

func ExampleUnauthorized_custom() {
	api := routes.NewBlueprint(routes.DefaultAuth(routes.AuthRequired))
	api.Get("/account", "", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h, err := routes.NewHandler(api.Routes(), routes.HandlerOptions{
		NotFound: response.NotFound(),
		Unauthorized: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response.WriteJSONError(w, http.StatusUnauthorized, "session_expired", "please sign in again")
		}),
	})
	if err != nil {
		panic(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/account", nil))
	fmt.Println(w.Code, w.Body.String())
	// Output:
	// 401 {"code":"session_expired","message":"please sign in again"}
}

func quotaExceeded(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", "60")
	response.WriteJSONError(w, http.StatusTooManyRequests, "export_quota_exceeded", "try again in one minute")
}

func ExampleWriteJSONError_customHandler() {
	h := middleware.RateLimit(middleware.RateLimitOptions{
		Requests: 1,
		Window:   time.Minute,
		OnLimit:  quotaExceeded,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for range 2 {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/exports", nil))
		fmt.Println(w.Code)
		if w.Code == http.StatusTooManyRequests {
			fmt.Println("Retry-After:", w.Header().Get("Retry-After"))
			fmt.Println(w.Body.String())
		}
	}
	// Output:
	// 204
	// 429
	// Retry-After: 60
	// {"code":"export_quota_exceeded","message":"try again in one minute"}
}
