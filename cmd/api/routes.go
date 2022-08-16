package main

import (
	"expvar"
	"github.com/julienschmidt/httprouter"
	"net/http"
)

func (app *application) routes() http.Handler {
	// Initialize a new httprouter router instance
	router := httprouter.New()

	// Convert the notFoundResponse() helper to a http.Handler using the http.HandlerFunc() adapter, and then set it as the custom error handler for 404
	// Not Found responses
	router.NotFound = http.HandlerFunc(app.notFoundResponse)

	// Register the relevant methods, URL patterns and handler functions for our endpoints using HandlerFunc() method .
	// Note that http.MethodGet and http.MethodPost are constants which equate to the strings "GET" and "POST" respectively
	//Likewise, convert the methodNotAllowedResponse() helper to a http.Handler and set it as
	//the  custom error handler for 405 Method Not Allowed responses

	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)
	// Add the route for the GET /v1/movies endpoint
	router.HandlerFunc(http.MethodGet, "/v1/movies", app.requirePermission("movies:read", app.listMoviesHandler))
	router.HandlerFunc(http.MethodPost, "/v1/movies", app.requirePermission("movies:write", app.createMovieHandler))
	router.HandlerFunc(http.MethodGet, "/v1/movies/:id", app.requirePermission("movies:read", app.showMovieHandler))
	router.HandlerFunc(http.MethodPatch, "/v1/movies/:id", app.requirePermission("movies:write", app.updateMovieHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/movies/:id", app.requirePermission("movies:write", app.deleteMovieHandler))
	// Add the route for the POST /v1/users endpoint
	router.HandlerFunc(http.MethodPost, "/v1/users", app.registerUserHandler)
	router.HandlerFunc(http.MethodPut, "/v1/users/activated", app.activateUserHandler)
	// Add the route for rhe POST /v1/tokens/authentication endpoint
	router.HandlerFunc(http.MethodPost, "/v1/tokens/authentication", app.createAuthenticationTokenHandler)

	//Register a new GET /debug/vars   endpoint   pointing to the expvar handler
	router.Handler(http.MethodGet, "/debug/vars", expvar.Handler())

	// Return the httprouter instance
	return app.metrics(app.recoverPanic(app.enableCORS(app.rateLimit(app.authenticate(router)))))
}
