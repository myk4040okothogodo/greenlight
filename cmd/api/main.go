package main

import (
    "sync"
    "context"
    "expvar"
    "database/sql"
    "flag"
    "os"
    "runtime"
    "time"
    "strings"
    //import the pq driver so that it can register itself with the database/sql package. Note that we alias this import to the blank identifier, to stop the Go
    //compiler complaining that the package isnt being used.
    "github.com/myk4040okothogodo/greenlight/internal/data"
    "github.com/myk4040okothogodo/greenlight/internal/jsonlog"
    "github.com/myk4040okothogodo/greenlight/internal/mailer"
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

    // Add a new limiter struct containing fields for the request-per-second and burst values, and a boolean field which we can use to enable/disable rate limiting 
    // altogether
    limiter struct {
        rps       float64
        burst     int
        enabled   bool
    }
    smtp struct {
        host      string
        port      int
        username  string
        password  string
        sender    string
    }
    // Add a cors struct and trustedOrigins field with the type []string
    cors struct {
        trustedOrigins  []string
    }
}

// Define an applicaction struct to hold the dependencies for our HTTP handlers, helpers, and middleware. At the moment this only
// contains a copy of the config struct and a logger, but it will grow to include a lot more as our build progresses.
// Add a models fields to hold our new Models struct
type application struct {
    config config
    logger *jsonlog.Logger
    models data.Models
    mailer mailer.Mailer
    wg     sync.WaitGroup

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


    //Create command -line flags to read the setting values into the config struct. Notice that we use true as the default for the "enabled" setting?
    //
    flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 2, "Rate limiter maximum requests per second")
    flag.IntVar(&cfg.limiter.burst, "limiter-burst", 4, "Rate limiter maximum burst")
    flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")

    flag.StringVar(&cfg.smtp.host, "smtp-host", "smtp.mailtrap.io", "SMTP host")
    flag.IntVar(&cfg.smtp.port, "smtp-port", 25, "SMTP port")
    flag.StringVar(&cfg.smtp.username, "smtp-username", "9172343c191b6f" ,"SMTP username")
    flag.StringVar(&cfg.smtp.password,"smtp-password", "e94d6f1b9e1213" , "SMTP password")
    flag.StringVar(&cfg.smtp.sender, "smtp-sender", "Greenlight <no-reply@greenlight>","SMTP sender")

    // Use the flag.Func() function to process the -cors-trusted-origins command line flag. In this we use the strings.Fields() function to split
    // the flag value into a slice based on whitespace characters and assign it to our config struct. Importantly, if the -cors-trsusted-origins flag
    // is not present, contains the empty string , or contains only whitespace, then strings.Fields() will return and empty []string slice
    //
    flag.Func("cors-trusted-origins", "Trusted CORS origins (space separated)",  func(val string) error{
        cfg.cors.trustedOrigins = strings.Fields(val)
        return nil
    })
    flag.Parse()

    //Initialize a new jsonlog.logger which writes any messages *at or above* the INFO severity level to the standard out stream.
    logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)



    // Call the openDB() helper function to create the connection pool, passing in  the config struct. If this returns an error, we log it and exit  the application 
    // immediately
    db, err := openDB(cfg)
    if err != nil {
        //Use the PrintFatal() method to write a log entry containing  the error at the FATAL level and exit. we have no additional properties to include in the log
        //entry, so we pass nil as the second parameter.
        logger.PrintFatal(err, nil)
    }

    //Defer a call to db.close() so that the connection pool is closed before the main() function  exits.
    defer db.Close()

    //Also log a message to say that the connection pool has been succesfully established.
    logger.PrintInfo("database connection pool established", nil)

    //Publish a new "version" variable in the expvar handler containing our application version number (currently the constant "1.0.0")
    expvar.NewString("version").Set(version)

    //Publish the database connection pool statistics
    expvar.Publish("goroutines", expvar.Func(func() interface{}{
        return runtime.NumGoroutine()
    }))

    //Publish the database  connection pool statistics
    expvar.Publish("database", expvar.Func(func()  interface{}{
        return db.Stats()
    }))

    // Publish the current Unix timestamp
    expvar.Publish("timestamp", expvar.Func(func() interface{}{
        return time.Now().Unix()
    }))

    //Declare an instance of the application struct, containing the config struct and the logger .
    app := &application{
        config: cfg,
        logger: logger,
        models: data.NewModels(db),
        mailer: mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender),
    }

    
    //call app.serve() to start the server
    err = app.serve()
    if err != nil {
        logger.PrintFatal(err, nil)
    }
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
