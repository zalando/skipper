package flowid

// flowIdGenerator interface should be implemented by types that can generate request tracing Flow IDs.
type flowIDGenerator interface {
	// Generate returns a new Flow ID using the implementation specific format or an error in case of failure.
	Generate() (string, error)
	// MustGenerate behaves like Generate but panics on failure instead of returning an error.
	MustGenerate() string
}
