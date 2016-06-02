package ping

import (
	"bytes"
	"fmt"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("pinger")

// A Status is returned by a Pinger to indicate the health of the pinged
// endpoint.
type Status int

const (
	// StatusUnknown indicates an endpoint that has not yet been pinged.
	StatusUnknown = iota
	// StatusOK indicates a endpoint that was successfully pinged.
	StatusOK Status = iota
	// StatusNOK indicates an endpoint that was not pinged successfully.
	StatusNOK = iota
)

var (
	statusStrings = []string{
		StatusUnknown: "UNKNOWN",
		StatusOK:      "OK",
		StatusNOK:     "NOK"}
)

// A Result represents the current status of a pinged endpoint. The Error
// field will be nil if the Status is StatusOK. If the Status is StatusNOK
// the Error field will contain further details on what went wrong.
// If the ping produced any output the Output field *may* contain output
// from the ping attempt.
type Result struct {
	Status Status
	Error  error
}

// A Pinger interface implementation contacts a single endpoint according to
// a certain protocol (such as a HTTP request or an SSH command) and returns
// a PingResult that indicates if the contacted endpoint gave an acceptable
// response (which also depends on the implementated ping mechanism).
type Pinger interface {
	// Ping contacts an endpoint according to a ping protocol and returns
	// a PingResult indicating the health of the endpoint. If the endpoint
	// gave an acceptable response, a StatusOK Status will be returned
	// (and a nil Error). If the endpoint failed to respond properly (or
	// could not be contacted), a StatusNOK/ is returned and an error is
	// returned with details on what went wrong.
	//
	// If supported by the pinger, any output produced by the ping may
	// also be returned (otherwise, output will be nil).
	//
	// Note: all details regarding the protocol, what denotes an
	// acceptable response, and the endpoint to contact, needs to be
	// encoded in/passed to the Pinger implementation.
	Ping() (result Result, output *bytes.Buffer)
}

func (result Result) String() string {
	return fmt.Sprintf("{Status: %s, Error: %v}", result.Status, result.Error)
}

func (status Status) String() string {
	return fmt.Sprintf("%s", statusStrings[status])
}
