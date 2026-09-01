package rbachttp

import (
	"net/http"

	"github.com/Ithildur/EiluneKit/http/response"
	"github.com/Ithildur/EiluneKit/http/routes"
)

const bearerSecurityName = "BearerAuth"

var errorResponseSchema = routes.SchemaOf[response.ErrorResponse]("ErrorResponse")

func addErrorResponses(opts []routes.RouteOption, statuses ...int) []routes.RouteOption {
	for _, status := range statuses {
		opts = append(opts, routes.JSONResponse(status, http.StatusText(status), errorResponseSchema))
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
