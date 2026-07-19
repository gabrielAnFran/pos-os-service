package dto

// Problem is an RFC 7807 problem+json error body.
type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func NewProblem(status int, title, detail, instance string) Problem {
	return Problem{
		Type:     "about:blank",
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	}
}
