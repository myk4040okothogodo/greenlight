package main

import (
    "fmt"
    "net/http"
    "time"
    "github.com/myk4040okothogodo/greenlight/internal/data"
)


// Add a createMovieHandler for the "POST /v1/movies" endpoint. For now we will simply return a plain-text placeholder response
//
func (app *application) createMovieHandler(w http.ResponseWriter, r * http.Request) {
    fmt.Fprintln(w, "create a new movie")
}


// Add a showMovieHandler for the "GET /vi/movies/:id" endpoint. For now, we retrieve the interpolated "id" parameter from the current URL and
// include it in  a placeholder response

func (app *application) showMovieHandler(w http.ResponseWriter, r *http.Request) {
    // When httprouter is parsing a request, an interpolated URL parameter will be stored in the request context. We can use the ParamsFromContext() function to 
    // retrieve a slice containing these parameters names and values.
    id, err := app.readIDParam(r)

    if err != nil {
        http.NotFound(w, r)
        return
    }
    
    // Create a new instance of the movie struct, containing the ID we extracted from the URL and some dummy data. Also notice that we deliberately haven set
    // a value for the year field.
    //
    movie := data.Movie{
        ID        :    id,
        CreatedAt :    time.Now(),
        Title     :    "Casablanca2",
        Runtime   :    102,
        Genres    :    []string{"Horror", "romance","war"},
        Version   :    1,
    }

    // Encode the struct to JSON and send it as the HTTP response
    err  = app.writeJSON(w, http.StatusOK, movie, nil)
    if err != nil {
        app.logger.Println(err)
        http.Error(w, "The server encountered a problem and couldnt process your request", http.StatusInternalServerError)
    }
}
