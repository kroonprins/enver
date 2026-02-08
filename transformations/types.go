package transformations

// Transformation is the interface that all transformations must implement
type Transformation interface {
	// Transform takes an input string and returns the transformed string
	Transform(input string) string
}

// Target specifies what the transformation applies to
type Target string

const (
	TargetKey   Target = "key"
	TargetValue Target = "value"
)
