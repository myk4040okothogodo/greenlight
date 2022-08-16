package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"github.com/myk4040okothogodo/greenlight/internal/validator"
	"time"
)

type Movie struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"-"`
	Title     string    `json:"title"`
	Year      int32     `json:"year,omitempty"`
	// Use the Runtime type instead of int32. Note that the omitempty directive will still work on this: if the Runtime field has the underlying value 0
	Runtime Runtime  `json:"runtime,omitempty"`
	Genres  []string `json:"genres,omitempty"` // Slice of genres for the movie (romance, comedy, etc)
	Version int32    `json:"version"`          // The version number starts at 1 and will be incremented each time the movie information is upadated
}

// The MovieModel struct type wraps a sql.DB connection pool.
type MovieModel struct {
	DB *sql.DB
}

type MockMovieModel struct{}

func ValidateMovie(v *validator.Validator, movie *Movie) {
	v.Check(movie.Title != "", "title", "must be provided")
	v.Check(len(movie.Title) <= 500, "title", "must not be more than 500 bytes long.")

	v.Check(movie.Year != 0, "year", "must be provided")
	v.Check(movie.Year >= 1888, "year", "must be greater than 1888")
	v.Check(movie.Year <= int32(time.Now().Year()), "year", "must not be the future")

	v.Check(movie.Runtime != 0, "runtime", "must be provided")
	v.Check(movie.Runtime > 0, "runtime", "must be a positive integer")

	v.Check(movie.Genres != nil, "genres", "must be provided")
	v.Check(len(movie.Genres) >= 1, "genres", "must be contain at least 1 genre")
	v.Check(len(movie.Genres) <= 5, "genres", "must not contain more than 5 genres")
	v.Check(validator.Unique(movie.Genres), "genres", "must not containe duplicate values")
}

//The Insert() method accepts a pointer to a movies struct, which should contain the data for the new record
func (m MovieModel) Insert(movie *Movie) error {
	//Define the SQL query for inserting a new record in the movies table and returning the system-generated data.
	query := `
        INSERT INTO movies (title, year, runtime, genres)
        VALUES ($1, $2, $3, $4)
        RETURNING id, created_at, version`

	//create an args slice containing the values for the placeholder parameters from the movie struct. Declaring this slice immediately next to our SQL query helps
	//to make it nice and clear *what values are being used where* in the query.
	args := []interface{}{movie.Title, movie.Year, movie.Runtime, pq.Array(movie.Genres)}

	//Create a context with a 3-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Use the QueryRow() method to execute the SQL query on our connection pool, passing in the args slice as a variadic parameter and scanning the system-generated
	// id, created_at and version values into the movie struct
	return m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.ID, &movie.CreatedAt, &movie.Version)
}

//Add a placeholder method for fetching a specific record from the movies tables.
func (m MovieModel) Get(id int64) (*Movie, error) {
	// The PostgreSQL bigserial type that we are using for the movie ID starts auto-incrementing at 1 by default,so we know that no movies will have ID values less
	// than that. To avoid making an unnecessary database call, we take a shorcut and return an ErrRecordNotFound error straight away.
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	// Define the SQL query for retrieving the movie data
	query := `
        SELECT id, created_at, title, year, runtime, genres, version 
        FROM movies
        WHERE id = $1`

	//Declare a Movie struct to hold the data returned by the query
	var movie Movie

	// Use the context.WithTimeout() function to create a context.Context which carries a 3-second timeout deadline. Note that we are using  the empty context.Background() as the aprent context
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	//Importantly, use defer to make sure that we cancel the context before the Get() method returns
	defer cancel()

	//Execute the query using the QueryRow() method, passing in the provided id value as a placeholder parameter, and scan the response data into the fields of the movie
	//struct. Importantly, notice that we need to convert the scan target for the genres column using the pq.Array() adapter function again.
	// Use the QueryRowContext() method to execute the query, passing in the context with the deadline as the first argument.
	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&movie.ID,
		&movie.CreatedAt,
		&movie.Title,
		&movie.Year,
		&movie.Runtime,
		pq.Array(&movie.Genres),
		&movie.Version,
	)

	//Handle any errors. If there was no matching movie found, Scan() will return a sql.ErrNoRows error. We check for this and return our custom ErrRecordNotFound
	//error instead
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	// Otherwise, return a  pointer to the Movie struct
	return &movie, nil
}

// Add a placeholder method for updating a specific record in the movies table.
func (m MovieModel) Update(movie *Movie) error {
	// Declare the SQL query for updating the record and returning the new version.
	query := `
        UPDATE movies
        SET title = $1, year = $2, runtime = $3, genres = $4, version = version + 1
        WHERE id = $5 AND version = $6
        RETURNING version`

	// Create an args slice containing the values for the placeholder paramerers
	args := []interface{}{
		movie.Title,
		movie.Year,
		movie.Runtime,
		pq.Array(movie.Genres),
		movie.ID,
		movie.Version,
	}

	//Create a context with a  3-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Use the QueryRow() method to execute the query, passing in the args slice as a variadic parrameter and scanning the new version value into the movie struct
	// Execute the SQL query. if no matching row could be found, we know the movie version has changed(or record has been deleted) and we return our custom ErrEditConflict error
	//
	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}

//Add a placeholder method for deleting a specific  record from the movies table.
func (m MovieModel) Delete(id int64) error {
	// Return an ErrRecordNotFound error if the movie ID is less than 1.
	if id < 1 {
		return ErrRecordNotFound
	}

	// Construct the SQL query to delete the record
	query := `
        DELETE FROM movies
        WHERE id = $1`

	// Create a context with 3-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Execute the SQL query using the Exec() method, passing in the id variable as the value for the placeholder parameter. The Exec() method returns a sql.Result object

	result, err := m.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	//call the RowsAffected() method on the sql.Result object to get the nmber or rows affected by the query
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	// If no rows were affected, we know that the movies table didnt contain a record with provided ID at the moment we tried to delete it. In that case we return an ErrRecordNotFound
	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

//Create a new GetAll() method which returns a slice of movies. Although we're not using the right now , we've set this to accept the various filter parameters as arguments
func (m MovieModel) GetAll(title string, genres []string, filters Filters) ([]*Movie, Metadata, error) {
	// Construct the SQL query to retrieve all movie records
	query := fmt.Sprintf(`
        SELECT count(*) OVER(), id, created_at, title, year, runtime, genres, version
        FROM movies
        WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', $1) OR $1 = '')
        AND (genres @> $2 OR $2 = '{}')
        ORDER BY %s %s, id ASC
        LIMIT $3 OFFSET $4`, filters.sortColumn(), filters.sortDirection())

	// Create a context with a 3-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// As our SQL query now has quite a few placeholder parameters, lets collect the values for the placeholders in a slice. Notice here how we call the limit() and offset()
	// methods on the Filters strcut to get the appropriate values for the LIMIT and OFFSET clauses
	args := []interface{}{title, pq.Array(genres), filters.limit(), filters.offset()}

	// Use QueryContext() to execute the query. This returns a sql.Rows resultset containing the result.
	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}
	// Importantly, defer a call to rows.Close() to ensure that the resultset is closed before GetAll() returns.
	defer rows.Close()
	// Initialize an empty slice to hold the movie data.
	totalRecords := 0
	movies := []*Movie{}
	// Use rows.Next to iterate through the rows in the resultset
	for rows.Next() {
		// Initialize an empty Movie struct to hold the data for an individual Movie.
		var movie Movie
		// Scan the values from the row into the Movie struct. Again, note that we're using the pq.Array() adapter on the genres field here
		err := rows.Scan(
			&totalRecords,
			&movie.ID,
			&movie.CreatedAt,
			&movie.Title,
			&movie.Year,
			&movie.Runtime,
			pq.Array(&movie.Genres),
			&movie.Version,
		)
		if err != nil {
			return nil, Metadata{}, err
		}
		// Add the movie struct to the slice
		movies = append(movies, &movie)
	}
	// When the rows.Next() loop has finished, call rows.Err() to retrieve any error that was encounteredd during the iteration
	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	// Generate a Metadata struct, passing in the total record count and pagination parameters from the client
	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	// If everything went OK, then return the slice of movies
	return movies, metadata, nil
}

func (m MockMovieModel) Insert(movie *Movie) error {
	// Mock the action
	return nil
}

func (m MockMovieModel) Get(id int64) (*Movie, error) {
	// Mock the action
	return nil, nil
}

func (m MockMovieModel) Update(movie *Movie) error {
	//Mock the action
	return nil
}

func (m MockMovieModel) Delete(id int64) error {
	// Mock the action
	return nil
}

func (m MockMovieModel) GetAll(title string, genres []string, filters Filters) ([]*Movie, Metadata, error) {
	return nil, Metadata{}, nil
}
