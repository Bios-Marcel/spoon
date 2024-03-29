package collections

func FlattenSlice[In any, Out any](slice []In, flatten func(value In) Out) []Out {
	out := make([]Out, 0, len(slice))
	for _, value := range slice {
		out = append(out, flatten(value))
	}
	return out
}
