package transformations

type Prefix struct {
	Value string
}

func (t *Prefix) Transform(input string) string {
	return t.Value + input
}
