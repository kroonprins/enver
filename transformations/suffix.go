package transformations

type Suffix struct {
	Value string
}

func (t *Suffix) Transform(input string) string {
	return input + t.Value
}
