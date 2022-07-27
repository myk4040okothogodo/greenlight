package main

import (
    "context"
    "database/sql"
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
    "time"
    //import the pq driver so that it can register itself with the database/sql package. Note that we alias this import to the blank identifier, to stop the Go
    //compiler complaining that the package isnt being used.
    "github.com/myk4040okothogodo/greenlight/internal/data"
    _ "github.com/lib/pq"
)



// Delcare a string containing the version number
const version = "1.0.0"

//Define a config struct to hold all the configuration settings for our application.
//For now, the only configuration settings will be the network port that we want the server to listen on 
//and the name of the current operating environment for the application(development, staging, production, etc). We
//will read in these con figuration setings from command-line flags when the application starts
//Add a db struct field to hold the configuration settings for our database connection pool. For now this only holds DSN,which we will read in from a command-line flag
//Add maxOpenConns, maxIdleConns and maxIdleTime fields to hole the configurtaion setting for the connection pool
type config struct {
    port int
    env string
    db struct {
        dsn            string
        maxOpenConns   int
        maxIdleConns   int
        maxIdleTime    string
    }
}

// Define an applicaction struct to hold the dependencies for our HTTP handlers, helpers, and middleware. At the moment this only
// contains a copy of the config struct and a logger, but it will grow to include a lot more as our build progresses.
// Add a models fields to hold our new Models struct
type application struct {
    config config
    logger *log.Logger
    models data.Models

}



func main(){
    //Declare an instance of the config struct
    var cfg config

    // Read the value of the port and env command-line flags into the config sruct. We default to using the port number 4000
    // and the environment "development" if no corresponding flags are provided.
    flag.IntVar(&cfg.port, "port", 4000, "API server port")
    flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")
    
    //  Read the DSN value from the db-dsn command-line flag into the config struct. We default to using our development DSN if no flag is provided.
    flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("GREENLIGHT_DB_DSN"), "PostgreSQL DSN")

    // Read the connection pool settings from command-line flags into the config struct Notice the default values we are using
    flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL max open connections")
    flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 25, "PostgreSQL max idle connections")
    flag.StringVar(&cfg.db.maxIdleTime, "db-max-idle-time", "15m", "PostgreSQL max connection idle time")
    flag.Parse()

    //Initialize a new logger which writes to the standard out stream
    //Prefixed with the current date and time
    logger := log.New(os.Stdout, "", log.Ldate | log.Ltime)



    // Call the openDB() helper function to create the connection pool, passing in  the config struct. If this returns an error, we log it and exit  the application 
    // immediately
    db, err := openDB(cfg)
    if err != nil {
        logger.Fatal(err)
    }

    //Defer a call to db.close() so that the connection pool is closed before the main() function  exits.
    defer db.Close()

    //Also log a message to say that the connection pool has been succesfully established.
    logger.Printf("database connection pool established")

    //Declare an instance of the application struct, containing the config struct and the logger .
    app := &application{
        config: cfg,
        logger: logger,
        models: data.NewModels(db),
    }

    // Declare a new servemux and add a /v1/healthcheck route which dispatches requests to the healthcheckHandler method
    // Declare a HTTP server with some sensible timeout settings, which listens om the port provided in the config struct and uses the servemux
    // we created above as the handler
    //
    srv := &http.Server{
        Addr    :                fmt.Sprintf(":%d", cfg.port),
        Handler :                app.routes(),
        IdleTimeout:             time.Minute,
        ReadTimeout:             10 * time.Second,
        WriteTimeout:            10 * time.Second,
    }


    logger.Printf("Starting %s server on %s ", cfg.env, srv.Addr)
    // Because the err variable is now already declared in the code above, we need to use the = operator instead of the := operator
    //
    err = srv.ListenAndServe()
    logger.Fatal(err)
}



// The openDB() function returns a sql.DB connection pool.
func openDB(cfg config) (*sql.DB, error){
    //use sql.open() to create an empty connection pool, using the DSN from the config struct
    db, err := sql.Open("postgres", cfg.db.dsn)
    if err != nil {
      return nil, err
    }


    // set the maximum number of open( in-use + idle ) connections in the pools. Note that passing a value less than or equal to 0 will mean there is no limit.
    db.SetMaxOpenConns(cfg.db.maxOpenConns)

    //Set the maximum number of idle connections in the pool. Again, passing a value less than or equal to 0 will mean there is no limit.
    db.SetMaxIdleConns(cfg.db.maxIdleConns)

    // Use the time.ParseDuration() function to convert the idle timeoout duration string to a time.Duration type.
    duration, err := time.ParseDuration(cfg.db.maxIdleTime)
    if err != nil {
        return nil, err
    }

    //Set the maximum idle timeout
    db.SetConnMaxIdleTime(duration)


    // Create a context with a 5-second timeout deadline.
    ctx,  cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    //Use pingContext() to establish a new connection to the database, passing in the context we created above as a parameter. If the connection couldn't be
    //established successfully within the 5 second deadline, then this will return an error.
    err = db.PingContext(ctx)
    if err != nil {
        return nil, err
    }

    // Return the sql.DB connection pool
    return db, nil
}
