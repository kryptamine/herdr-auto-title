// Package herdr implements a client for the Herdr local socket API: NDJSON over
// the socket named by HERDR_SOCKET_PATH, one request per connection.
// Verified against Herdr v0.8.2, protocol 20.
package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
)

// request is one outbound line. Herdr requires "params" on every method, so
// methods without parameters take an empty object rather than omitting it.
type request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

// frame is one inbound line: a result or an error.
type frame struct {
	Result json.RawMessage `json:"result"`
	Error  *APIError       `json:"error"`
}

// APIError is an error returned by Herdr for a request.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("herdr api error %s: %s", e.Code, e.Message)
}

// Error codes Auto Title reacts to.
const (
	// CodeTabNotFound is returned when a tab closed between the snapshot that
	// named it and the rename that followed.
	CodeTabNotFound = "tab_not_found"
	// CodePaneNotFound is the same for a pane, which can close between the
	// snapshot that listed it and the read of what is running in it.
	CodePaneNotFound = "pane_not_found"
)

// ErrorCode returns the Herdr error code carried by err, or "" if err is not a
// Herdr API error.
func ErrorCode(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}

	return ""
}

// Method names used by Auto Title: two to read the session and one to act on it.
const (
	MethodSessionSnapshot = "session.snapshot"
	MethodPaneProcessInfo = "pane.process_info"
	MethodTabRename       = "tab.rename"
)

// PaneTarget names the pane a request applies to.
type PaneTarget struct {
	PaneID string `json:"pane_id"`
}

// TabRenameParams are the parameters of tab.rename.
type TabRenameParams struct {
	TabID string `json:"tab_id"`
	Label string `json:"label"`
}

// emptyParams is the parameter object for methods that take none.
type emptyParams struct{}
