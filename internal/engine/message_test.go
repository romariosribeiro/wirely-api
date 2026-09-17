package engine

import "testing"

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "+55 (11) 99999-9999", want: "5511999999999"},
		{input: "351 912 345 678", want: "351912345678"},
	}
	for _, test := range tests {
		got, err := normalizePhone(test.input)
		if err != nil {
			t.Fatalf("normalizePhone(%q): %v", test.input, err)
		}
		if got != test.want {
			t.Fatalf("normalizePhone(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestNormalizePhoneRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{"1199abc", "123", "01234567890", "++5511999999999"} {
		if _, err := normalizePhone(input); err == nil {
			t.Fatalf("normalizePhone(%q) should fail", input)
		}
	}
}
