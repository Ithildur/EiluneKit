package authhttp

import (
	stdhttp "net/http"

	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"
)

const (
	bearerSecurityName        = "BearerAuth"
	refreshCookieSecurityName = "RefreshCookie"
	csrfCookieSecurityName    = "CSRFCookie"
	csrfHeaderSecurityName    = "CSRFHeader"
)

var errorResponseSchema = routes.SchemaOf[response.ErrorResponse]("ErrorResponse")

func addErrorResponses(opts []routes.RouteOption, statuses ...int) []routes.RouteOption {
	for _, status := range statuses {
		opts = append(opts, routes.JSONResponse(status, stdhttp.StatusText(status), errorResponseSchema))
	}
	return opts
}

func withBearerSecurity(opts []routes.RouteOption) []routes.RouteOption {
	return append(opts,
		routes.Security(routes.SecurityRequirement{
			{
				Name:   bearerSecurityName,
				Type:   routes.SecurityHTTP,
				Scheme: "bearer",
			},
		}),
	)
}

func withRefreshSecurity(opts []routes.RouteOption, options Options) []routes.RouteOption {
	return append(opts,
		routes.Security(routes.SecurityRequirement{
			{
				Name:          refreshCookieSecurityName,
				Type:          routes.SecurityAPIKey,
				ParameterName: options.RefreshCookieName,
				In:            routes.ParameterCookie,
			},
			{
				Name:          csrfCookieSecurityName,
				Type:          routes.SecurityAPIKey,
				ParameterName: options.CSRFCookieName,
				In:            routes.ParameterCookie,
			},
			{
				Name:          csrfHeaderSecurityName,
				Type:          routes.SecurityAPIKey,
				ParameterName: options.CSRFHeaderName,
				In:            routes.ParameterHeader,
			},
		}),
	)
}
