//go:generate easyjson -all ${GOFILE}

package json

//easyjson:json
type Matches []Match

type Match struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	Bucket      string `json:"bucket"`
	Version     string `json:"version"`
}

type App struct {
	Description string `json:"description"`
	Bin         any    `json:"bin"`
	Version     string `json:"version"`
}
