package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"github.com/myk4040okothogodo/greenlight/internal/validator"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Retrieve the "id" URL parameter from the current request context, then convert it to an integer and return it. If thhe operation isnt successful return o
// and an error
func (app *application) readIDParam(r *http.Request) (int64, error) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.ParseInt(params.ByName("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id parameter")
	}
	return id, nil
}

//Define an  envelope type
type envelope map[string]interface{}

//Define a writeJSON() helper for sending responses. This takes the destination  http.ResponseWriter, the HTTP status code to send, the data to encode to JSON and a
//header map containing any additional HTTP headers we want to include in the response
//
func (app *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error {
	// Encode the data to JSON, returning the error if there was one.
	//use MarshalIndent() function so that whitespace is added to the encoded JSON. Here we use no line prefix(" ") and tab indents ("\t")
	js, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	// Append a newline to make it easier to view in terminal applications.
	js = append(js, '\n')

	//At this point, we know that we wont encounter any more errors before writing the response, so its safe to add any headers that we want to include. We loop
	//through the header map and add each header to the http.ResponseWriter header map. Note that its OK if the provided header map is nil. Go doesnt throw an error
	//if you try to range over (or generally read from) a nil map.
	for key, value := range headers {
		w.Header()[key] = value
	}

	//Add the "Content-Type: application/json" header, then write the status code and JSON response.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil

}

func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {

	// Use http.MaxBytesReader() to limit the size of the request body to 1MB.
	maxBytes := 1_048_576
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
	// Initialize the json.Decoder, and call the DisallowUnknownFields() methods on it before decoding.
	// This means that if the JSON from the client now includes any field which cannot be mapped to the target , the decoder will retuen an error instead of just ignoring
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	// decode the request body to the destination
	err := dec.Decode(dst)
	if err != nil {
		//If there is an error during decoding, start the triage .....
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError

		switch {
		// Use the errors.As() function  to check whether the error has the type *json.SyntaxError. If it does, the return a plain-english error message which include
		// the location of the problem
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

			// In some circumstances Decode() may also return an io.ErrUnexpectedEOF error for syntax errors in the JSON. So we check for this using errors.Is() and return
			// a generic error message. There is an open issue regarding this matter
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")

		//Likewise, catch any *json.UnmarshalTypeError errors. These occur when the JSON value is the wrong type for the target destination. If the error relates to a specific
		//field, then we include that in our error message to make it easier for the client to debug.
		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)

			// An io.EOF error will be returned by Decode() if the request body is empty. we
			// check for this with errors.Is() and return a plain-english error message instead
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")

		//If the JSON contains a field which cannot be mapped to the target destination then Decode() will now return an error message in the format
		// "json: unknown field "<name>"". We check for this, extract the field name from the error, and interpolate it into our custome error message
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("body contains unknown key %s", fieldName)

		// If the request body exceeds 1MB in size thee decode will now fail with the error "http: request body too large"
		case err.Error() == "http: request body too large":
			return fmt.Errorf("body must not be larger than %d bytes", maxBytes)

		// A json.InvalidUnmarshalError error will be returned if we pass a non-nill pointer to Decode()
		// we catch this and panic, rather than returning an error to our handler.
		case errors.As(err, &invalidUnmarshalError):
			panic(err)

		default:
			return err
		}
	}

	//Call Decode() again, using a pointer to an empty enonymous struct as the destination. If the request body only contained a single jSON value this will return
	//an io.EOF error. SO if we get anything else, we know that there is additional data in the request body and we return our own custom error message
	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		return errors.New(" Body must only contain a single JSON value")
	}

	return nil
}

// The readString() helper returns a string value from the query string , or the provided default value if no matching key could be found.
func (app *application) readString(qs url.Values, key string, defaultValue string) string {
	// Extract the value for a given key from the query string. If no key exists this will return the empty string ""
	s := qs.Get(key)

	//If no key exists(or the value is empty). then return the default value.
	if s == "" {
		return defaultValue
	}

	// Otherwise return the string
	return s
}

// The readCSV() helper reads a string value from the query string and the splits it into a slice on the comma character. If no matching key could be found it returns
// the provided default value.

func (app *application) readCSV(qs url.Values, key string, defaultValue []string) []string {
	// Extract the value from the query string
	csv := qs.Get(key)

	//If no key exists (or the value is empty) then return the default value.
	if csv == "" {
		return defaultValue
	}

	//Otherwise parse the value into a []string slice and return it
	return strings.Split(csv, ",")
}

//The readInt() helper reads a string value from the query string and converts it to an integer before returning it. If no matching key could be found it returns the
//Provided default value. If the value couldnt be converted to an integer, then we record an error message in the provided validator instance.

func (app *application) readInt(qs url.Values, key string, defaultValue int, v *validator.Validator) int {
	// Extract the value from the query string
	s := qs.Get(key)

	// If no key exists (or the value is empty) the return the default value.
	if s == "" {
		return defaultValue
	}

	// Try to convert the value to an int. If this fails, add an error message to the validator instance and return the default value
	i, err := strconv.Atoi(s)
	if err != nil {
		v.AddError(key, "must be an integer value")
		return defaultValue
	}

	// Otherwise, return the converted integer value
	return i
}

// The background() helper accepts an arbitrary function as a parameter
func (app *application) background(fn func()) {

	// Increment the WaitGroup counter
	app.wg.Add(1)

	//Launch a background goroutine
	go func() {

		//use defer to decrement the WaitGroup counter before the goroutine returns
		defer app.wg.Done()

		// Recover any panic.
		defer func() {
			if err := recover(); err != nil {
				app.logger.PrintError(fmt.Errorf("%s", err), nil)
			}
		}()

		// Execute the arbitrary function that we passed as the parameter
		fn()

	}()
}
