package spaces

import "fmt"

// Error is a failure Spaces reported: the HTTP status and, when the
// response carried one, S3's error code and message (for example 403
// InvalidAccessKeyId), so a log line says why without parsing XML.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("spaces: %d %s: %s", e.Status, e.Code, e.Message)
}
