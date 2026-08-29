// Package pbconv holds helpers for converting domain models to protobuf
// messages in the gRPC handlers.
package pbconv

// Slice converts every element of items with conv. conv receives a pointer
// to the element so that pointer-receiver ToProtobufObject methods can be
// passed directly, e.g. Slice(marks, (*models.Mark).ToProtobufObject).
func Slice[T any, P any](items []T, conv func(*T) P) []P {
	result := make([]P, len(items))
	for i := range items {
		result[i] = conv(&items[i])
	}

	return result
}
