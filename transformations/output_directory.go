package transformations

// OutputDirectory sets the value to the output directory
type OutputDirectory struct {
	Directory string
}

func (t *OutputDirectory) Transform(input string) string {
	return t.Directory
}
