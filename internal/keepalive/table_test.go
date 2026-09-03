package keepalive

import "testing"

func TestQuoteTable(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"public.users", `"public"."users"`},
		{"users", `"users"`},
		{"  public.keepalive_ping  ", `"public"."keepalive_ping"`},
		{"_private.t$1", `"_private"."t$1"`},
	} {
		got, err := QuoteTable(tc.in)
		if err != nil {
			t.Errorf("QuoteTable(%q) returned error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("QuoteTable(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestQuoteTableRejectsAnythingElse(t *testing.T) {
	for _, in := range []string{
		"users; drop table secrets",
		"users--",
		"public.users, other",
		"my table",
		`"users"`,
		"users)",
		"1users",
		"public.users.extra",
		"",
		"   ",
		"'users'",
		"public.",
	} {
		if got, err := QuoteTable(in); err == nil {
			t.Errorf("QuoteTable(%q) = %q, want an error", in, got)
		}
	}
}
