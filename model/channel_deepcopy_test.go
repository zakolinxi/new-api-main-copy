package model

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestChannelDeepCopyDoesNotAliasSharedStorage guards DeepCopy against a newly
// added pointer, map, or slice field. Channel updates decode the request body
// over a copy of the persisted row, and json decoding writes through non-nil
// pointers, so a field that DeepCopy forgets to clone silently lets the request
// overwrite the origin snapshot the same request is authorized against.
func TestChannelDeepCopyDoesNotAliasSharedStorage(t *testing.T) {
	source := &Channel{}
	sourceValue := reflect.ValueOf(source).Elem()
	for i := 0; i < sourceValue.NumField(); i++ {
		field := sourceValue.Field(i)
		if !field.CanSet() {
			continue
		}
		switch field.Kind() {
		case reflect.Ptr:
			field.Set(reflect.New(field.Type().Elem()))
		case reflect.Map:
			field.Set(reflect.MakeMapWithSize(field.Type(), 1))
		case reflect.Slice:
			field.Set(reflect.MakeSlice(field.Type(), 1, 1))
		}
	}
	source.ChannelInfo = ChannelInfo{
		MultiKeyStatusList:     map[int]int{0: 1},
		MultiKeyDisabledReason: map[int]string{0: "disabled"},
		MultiKeyDisabledTime:   map[int]int64{0: 1},
	}

	cloned := source.DeepCopy()
	require.NotSame(t, source, cloned)
	assertNoSharedStorage(t, "Channel", reflect.ValueOf(source).Elem(), reflect.ValueOf(cloned).Elem())
}

func assertNoSharedStorage(t *testing.T, path string, source, cloned reflect.Value) {
	t.Helper()
	switch source.Kind() {
	case reflect.Ptr:
		if source.IsNil() {
			return
		}
		require.False(t, cloned.IsNil(), "%s must stay non-nil after DeepCopy", path)
		if source.Pointer() == cloned.Pointer() {
			t.Errorf("%s still points at the source allocation; DeepCopy must clone it", path)
			return
		}
		assertNoSharedStorage(t, path, source.Elem(), cloned.Elem())
	case reflect.Map:
		if source.IsNil() {
			return
		}
		if source.Pointer() == cloned.Pointer() {
			t.Errorf("%s still shares the source map; DeepCopy must clone it", path)
		}
	case reflect.Slice:
		if source.Len() == 0 {
			return
		}
		if source.Pointer() == cloned.Pointer() {
			t.Errorf("%s still shares the source backing array; DeepCopy must clone it", path)
		}
	case reflect.Struct:
		for i := 0; i < source.NumField(); i++ {
			field := source.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			assertNoSharedStorage(t, path+"."+field.Name, source.Field(i), cloned.Field(i))
		}
	}
}
