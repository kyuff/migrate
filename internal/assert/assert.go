package assert

import (
	"cmp"
	"reflect"
	"strings"
	"testing"
)

func Equal[T comparable](t testing.TB, want, got T) {
	t.Helper()
	if want != got {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func GreaterOrEqual[T cmp.Ordered](t testing.TB, got, floor T) {
	t.Helper()
	if got < floor {
		t.Fatalf("expected %v to be >= %v", got, floor)
	}
}

func LessOrEqual[T cmp.Ordered](t testing.TB, got, ceiling T) {
	t.Helper()
	if got > ceiling {
		t.Fatalf("expected %v to be <= %v", got, ceiling)
	}
}

func Len(t testing.TB, got any, want int) {
	t.Helper()

	v := reflect.ValueOf(got)
	switch v.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		if v.Len() != want {
			t.Fatalf("expected len %d, got %d", want, v.Len())
		}
	default:
		t.Fatalf("len is not supported for type %T", got)
	}
}

func NoError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func Error(t testing.TB, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
}

func Truef(t testing.TB, cond bool, format string, args ...any) {
	t.Helper()
	if !cond {
		t.Fatalf(format, args...)
	}
}

func False(t testing.TB, cond bool) {
	t.Helper()
	if cond {
		t.Fatalf("expected false, got true")
	}
}

func Contains(t testing.TB, got, needle string) {
	t.Helper()
	if !strings.Contains(got, needle) {
		t.Fatalf("expected %q to contain %q", got, needle)
	}
}
