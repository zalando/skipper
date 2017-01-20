package flowid

// Generator interface should be implemented by types that can generate request tracing Flow IDs.
type Generator interface {
	// Generate returns a new Flow ID using the implementation specific format or an error in case of failure.
	Generate() (string, error)
	// MustGenerate behaves like Generate but panics on failure instead of returning an error.
	MustGenerate() string
	// IsValid asserts if a given flow ID follows an expected format
	IsValid(string) bool
}
