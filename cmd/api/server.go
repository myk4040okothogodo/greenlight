package main

import (
    "context"
    "errors"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
)


func (app *application) serve() error {
    //Declare a HTTP server using the same settings as in our main() function.
    //
    srv := &http.Server{
        Addr    :          fmt.Sprintf(":%d", app.config.port),
        Handler :          app.routes(),
        IdleTimeout:       time.Minute,
        ReadTimeout:       10 * time.Second,
        WriteTimeout:      30 * time.Second,
    }


    //Create a shutdown channel. We will use this to receive any errors returned  by graceful shutdown() function
    shutdownError := make(chan error)

    go func() {
        // Intercept the signals, as before
        quit := make(chan os.Signal, 1)
        signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
        s := <-quit

        // Update the log entry to say "shutdown down server" instead of "caught signal"
        app.logger.PrintInfo("shutting down server", map[string]string{
            "signal": s.String(),
        })

        // Create a context with a 5-second timeout
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        // Call shutdown() on our server, passing in the context we just made. Shutdown() will return nill if graceful shutdown was successful.
        // or and error( which may happen because of a problem closing the listeners, or because the shutdown ddidnt complete before the 5-second
        // context deadline is hit). We relay this return value to the shutdownError channel
        err := srv.Shutdown(ctx)
        if err != nil {
            shutdownError <- err
        }

        // Log a message to say that we are waitin for any background goroutines to complete their tasks,
        app.logger.PrintInfo("Completing background tasks", map[string]string{
            "addr": srv.Addr,
        })
        // Call wait() to blocck untill our WaitGroup counter is zero   --- essentially blocking until the background goroutines have finished . Then we
        // return nil on the shutdownError channel, to indictate that the shutdown completed withour any issues.
        app.wg.Wait()
        shutdownError <- nil
    }()

    app.logger.PrintInfo("starting server ", map[string]string{
        "addr": srv.Addr,
        "env":  app.config.env,
    })


    //calling shutdown() on our server will cause ListenAndServe() to immediately  return aa http.ErrServerClosed error. So if we see this error, it is
    //actually a good thing and an indication that graceful shutdown has started. So we check specifically for this , only returning the error if its not
    //http.ErrServerClosed
    //
    err := srv.ListenAndServe()
    if !errors.Is(err, http.ErrServerClosed){
        return err
    }

    //otherwise, we wait to receive the return value from shutdown() on the shutdown channel. If return value is an error, we  know that there was a problem
    //with the graceful shutdonw and we return the error
    err = <-shutdownError
    if err != nil {
        return err
    }

    // At this point we know that the graceful shutdown completed successfully and we log a "stopped server" message.
    app.logger.PrintInfo("Stopped server", map[string]string{
        "addr": srv.Addr,
    })

    return nil
}
