package main

import (
    "fmt"
    "net"
    "net/http"
    "sync"
    "time"
    "golang.org/x/time/rate"
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
