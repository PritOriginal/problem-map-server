// Package reflectutil holds small reflection helpers shared across packages.
package reflectutil

import "reflect"

// IsNil reports whether v is nil, including a typed nil (e.g. a nil *T or
// nil map) stored in an interface, which a plain "v == nil" misses.
func IsNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface, reflect.UnsafePointer:
		return rv.IsNil()
	default:
		return false
	}
}
