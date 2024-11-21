package json

import (
	"io"

	jsoniter "github.com/json-iterator/go"
)

func ResetBytes(iter *jsoniter.Iterator, bytes []byte) {
	iter.ResetBytes(bytes)
	// Workaround for the fact Reset doesn't do this
	iter.Error = nil
}

func Reset(iter *jsoniter.Iterator, reader io.Reader) {
	iter.Reset(reader)
	// Workaround for the fact Reset doesn't do this
	iter.Error = nil
}
