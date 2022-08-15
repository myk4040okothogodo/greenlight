package main

import (
    "errors"
    "expvar"
    "fmt"
    "net"
    "net/http"
    "strconv"
    "strings"
    "sync"
    "time"
    "golang.org/x/time/rate"
    "github.com/myk4040okothogodo/greenlight/internal/data"
    "github.com/myk4040okothogodo/greenlight/internal/validator"
    "github.com/felixge/httpsnoop"
)


func (app *application) recoverPanic(next http.Handler) http.Handler {
    return http.HandlerFunc(func (w http.ResponseWriter, r *http.Request){
       // Create a deferred function (which will always be run in the event of a panic as Go unwinds the stack)
       defer func() {
           // Use the built-in recover function to check if there has been a ppanic or not.
           if err := recover(); err != nil {
               // If there was a panic, set a "Connection: close" header on the response. This acts as a trigger to make Go's HTTP server autommatically close
               // the curren connection after a response has been sent
               w.Header().Set("Connection ", "close")

               // The value returned by recover() has the type interface{}, so we use fmt.Errorf() to normalize it into an error and call our serverErrorResponse()
               // helper. In turn, this will log the error using our custom Logger type at the ERROR level and send the client a 500 Internal Server Error response.
               app.serverErrorResponse(w, r, fmt.Errorf("%s", err))
           }
       }()

       next.ServeHTTP(w, r)
    })
}


func (app *application) rateLimit(next http.Handler) http.Handler {


    //Define a client struct to hold the rate limiter and last seen time for each client.
    type client struct {
        limiter  *rate.Limiter
        lastSeen time.Time
    }
    /// Declare a mutex and a map to hold the clients' IP addresses and rate limiters

    var (
        mu                sync.Mutex
        clients   = make(map[string]*client)
    ) 


    // Launch a background goroutine whcih removes old entries from the clients map once every minute
    go func() {
        for {
            time.Sleep(time.Minute)

            //Lock the mutex to prevent any rate limiter checks from happening while the cleanup is taking place
            mu.Lock()

            //Loop through all clients. If they havent been seen within the last three minutes, delete the corresponding entry from the map.
            for ip, client := range clients {
                if time.Since(client.lastSeen) > 3 * time.Minute {
                    delete(clients, ip)
                }
            }

            // Importantly, unlock the mutex when the cleanup is complete
            mu.Unlock()
        }
    }()

    // The function we are returning is a closure , which "closes over" the limiter variable.
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){


        // Only carry out the check if rate limiting is enabled
        // Call limiter.Allow() to see if the request is permitted, and if its not, then we call the rateLimitExceededResponse() helper to return a 429 Too Many request
        // response (we will create this helper in a minute)
        //
        //Extract the clients IP address from the request.
        if app.config.limiter.enabled {
        ip, _, err := net.SplitHostPort(r.RemoteAddr)
        if err != nil {
            app.serverErrorResponse(w, r, err)
            return
        }

        //Lock the mutex to prevent this code from being executed concurrently.
        mu.Lock()

        // Check to see if the IP addresses already exists in the map. If it doesnt then  initialize a new rate limiter and add the IP address and limiter to the map
        //Create and add a new client struct to the map if id doenst already exist.

        if _, found := clients[ip]; !found {
          clients[ip] = &client{
             // use the requests-per-second and burst values from the config struct
             limiter: rate.NewLimiter(rate.Limit(app.config.limiter.rps), app.config.limiter.burst),
           }
        
        }
        // Update the last seen time for the client.
        clients[ip].lastSeen  = time.Now()

        //call the Allow() method on the rate limiter for rhe currentt IP address. if the request isnt allowed, unlock the mutex and send a 429 Too Many Requests response
        //, just like before
        if !clients[ip].limiter.Allow() {
            mu.Unlock()
            app.rateLimitExceededResponse(w, r)
            return
        }


        // Very importantly, unlock the mutex before calling the next handler un the chain. Notice that we dont use defer to unlock the mutex, as that would mean that
        // the mutex isnt unlocked untill all the handler downstream of this middlewware have also returned .
        mu.Unlock()
      }
        next.ServeHTTP(w, r)
    })
}



func (app *application) authenticate(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
        // Add the "Vary: Authorization" header to the response. This indicates to any caches that the response may vary based on the value of the AUthorization header in
        // the request
        w.Header().Add("Vary", "Authorization")

        //Retrieve the value of the Authorization header from the request. This will return the empty string "" if there is no such header found.
        authorizationHeader := r.Header.Get("Authorization")


        //If there is no Authorization header found, use the contextSetUser() helper that we just made to add the AnonymousUsser to the request context. Then we call the
        //next handler in the chain and return withou executing any of the code below.
        //
        //
        if authorizationHeader == "" {
          r = app.contextSetUser(r, data.AnonymousUser)
          next.ServeHTTP(w, r)
          return 
        }

        // Otherwise, we expect the value of the Authorization header to be in the format  "Bearer <token>". We try to split this into its constituent parts, and if the header
        // isnt in the expected format we return a 401 Unauthorized response using the invalidAuthenticationTokenResponse() helper (which we will create ina a moment.)
        // 
        headerParts := strings.Split(authorizationHeader, " ")
        if len(headerParts) != 2 || headerParts[0] != "Bearer" {
           app.invalidAuthenticationTokenResponse(w, r)
           return
        }

        //Extract the actual authentication token from the header parts.
        //
        token := headerParts[1]

        //Validate the token to make sure it is in a sensible format.
        v := validator.New()

        // If the token isnt valid, use the invalidAuthenticationTokenResponse()
       // hepler to send a response , rather than the failedValidationResponse() helper that wed normally use
        if data.ValidateTokenPlaintext(v, token); !v.Valid() {
          app.invalidAuthenticationTokenResponse(w, r)
          return
        }


        // Retrieve the details of the user associated withe the authentication token, again calling the invalidAuthenticationTokenResponse() helper
        // iff no matching record was found. IMPORTANT: Notice that we are using ScopeAuthentication as the first parameter here.
        //
        user, err :=  app.models.Users.GetForToken(data.ScopeAuthentication, token)
        if err != nil {
          switch {
          case errors.Is(err, data.ErrRecordNotFound):
            app.invalidAuthenticationTokenResponse(w, r)
          default:
            app.serverErrorResponse(w, r, err)
          }
          return
        }

        //call the contextSetUser() helper to add the user information to the request context
        r = app.contextSetUser(r, user)

        //call the next handler in the chain
        next.ServeHTTP(w, r)
      })
}


// Create a new requiredAuthenticatedUser() middleware to check that a user is not anonymous.
func (app *application) requireAuthenticatedUser(next http.HandlerFunc) http.HandlerFunc {
    return http.HandlerFunc(func(w http.ResponseWriter,  r *http.Request){
      user := app.contextGetUser(r)

      if user.IsAnonymous(){
          app.authenticationRequiredResponse(w, r)
          return
      }
      next.ServeHTTP(w, r)
    })
}


//Checks that a user is both authenticated and activated.
func (app *application) requireActivatedUser(next http.HandlerFunc) http.HandlerFunc {
    // Rather than returning this http.HandlerFunc we assign it to the variable fn.
    fn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
        user := app.contextGetUser(r)

        // Check that a user is activated
        if !user.Activated {
            app.inactiveAccountResponse(w, r)
            return
        }

        next.ServeHTTP(w, r)
    })

    //Wrap fn with the requireAuthenticatedUser() middleware before returning it.
    return app.requireAuthenticatedUser(fn)
}


// Note that the first parameter for the middleware function is the permission code that we require the user to have
func (app *application) requirePermission(code string, next http.HandlerFunc) http.HandlerFunc {
    fn := func(w http.ResponseWriter, r *http.Request){
        // retrieve the user from the request context.
        user := app.contextGetUser(r)

       // Get the slice of permissions for the user.
       permissions, err := app.models.Permissions.GetAllForUser(user.ID)
       if err != nil {
           app.serverErrorResponse(w, r, err)
           return
       }

       //check if the slice includes the required permission. If it doesnt, then return a 403 Forbidden response.
       if !permissions.Include(code){
           app.notPermittedResponse(w, r)
           return
       }

       // Otherwise they have the required permission so we call the next handler in the chain
       //
       next.ServeHTTP(w, r)
    }

    //Wrap this with the requireActivatedUser() middleware before returning it
    return app.requireActivatedUser(fn)
}

func (app *application) enableCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
      //Add the "Vary: Origin" header
      w.Header().Add("Vary", "Origin")

      //Add the "vary: Access-Control-Request-Method" header
      w.Header().Add("Vary", "Access-Control-Request-Method")

      origin := r.Header.Get("Origin")

      // Only run this if there's an Origin request header present AND at least on trusted origin is configured
      if origin !=  " " && len(app.config.cors.trustedOrigins) != 0 {
         // Loop through the list of trusted origins, checking to see if the request
         // origin exactly matches one of them.
         for i := range app.config.cors.trustedOrigins {
            if origin == app.config.cors.trustedOrigins[i]{
                // If there is a match, then set a "Access-control-Origins"
                // Reponse header with the request origin as the value
                w.Header().Set("Access-Control-Allow-Origin", origin)

                //Check if the request has thee HTTP method OPTIONS and contains the "Access-Control-Request-Method" header. If it does,
                //then we treat it as a preflight request.
                if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-method") != "" {
                    /// set the necessary preflight response headers, as discussed previously

                    w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, PUT, PATCH, DELETE")
                    w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

                    // Write the headers along with a 200 OK status and return from the middleware with no further action
                    //
                    w.WriteHeader(http.StatusOK)
                    return
                }
            }
         }
      }

      next.ServeHTTP(w,r)
    })
}



func (app *application) metrics(next http.Handler) http.Handler {
    // Initialize the new exxpvar variables when the middleware chain is first built.
    totalRequestsReceived :=  expvar.NewInt("total_requests_received") 
    totalResponsesSent    :=  expvar.NewInt("total_responses_sent")
    totalProcessingTimeMicroseconds := expvar.NewInt("total_processing_time_milisecons")
  
    // Declare a new expvar map to hold the count of responses for each HTTP statue code
    totalResponsesSentByStatus :=  expvar.NewMap("total_responses_sent_by_status")

    //The following code will be run for every request
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){

      // Increment the requests received count, like before.
      totalRequestsReceived.Add(1)


      // Call the httpsnoop.CaptureMetrics() function, passing in the next handler in the chain along with the
      // existing http.ResponseWriter and http.Request. This returns the metrics struct that we saw above.
      //
      metrics  := httpsnoop.CaptureMetrics(next, w, r)


      //Increment the response sent count , like before.
      totalResponsesSent.Add(1)


      //Get the request processing time in microseconds from httpsnoop and increamentt the cumulative processing time
      //
      totalProcessingTimeMicroseconds.Add(metrics.Duration.Microseconds())

      // Use the Add() method to increment the count for the given status code by 1. Note that the expvar map in string-keyed,
      // so we need to use the strconv.Itoa() function to convert the status code(which is an integer) to a string.
      //
      totalResponsesSentByStatus.Add(strconv.Itoa(metrics.Code), 1)
    })
}
