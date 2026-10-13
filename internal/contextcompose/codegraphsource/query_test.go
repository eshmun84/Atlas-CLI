package codegraphsource

import "testing"

func TestDeriveQuerySymbol(t *testing.T) {
	t.Parallel()
	cases := []struct {
		task string
		want string
		ok   bool
	}{
		{"explain helper function callers", "helper", true},
		{"inspect ProcessPayment", "ProcessPayment", true},
		{"look at snake_case_name usage", "snake_case_name", true},
		{"please explain the function callers", "", false},
		{"", "", false},
		{"review FooBar and helper", "FooBar", true},
	}
	for _, tc := range cases {
		got, ok := deriveQuerySymbol(tc.task)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("task=%q got=(%q,%v) want=(%q,%v)", tc.task, got, ok, tc.want, tc.ok)
		}
	}
}
