package data

import (
    "time"
)

type Movie struct {
  ID          int64        `json: "id"`  
  CreatedAt   time.Time    `json: "-"`   
  Title       string       `json: "title"`  
  Year        int32        `json: "year"omitempty`  
  // Use the Runtime type instead of int32. Note that the omitempty directive will still work on this: if the Runtime field has the underlying value 0
  Runtime     Runtime       `json: "runtime,omitempty` 
  Genres      []string     `json : "genres,omitempty"`   // Slice of genres for the movie (romance, comedy, etc)
  Version     int32        `json: "version"`   // The version number starts at 1 and will be incremented each time the movie information is upadated
}
