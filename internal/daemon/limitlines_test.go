package daemon

import "testing"

func TestLimitLines(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		head   int
		tail   int
		expect string
	}{
		{"empty", "", 5, 0, ""},
		{"head 3", "a\nb\nc\nd\ne", 3, 0, "a\nb\nc"},
		{"tail 3", "a\nb\nc\nd\ne", 0, 3, "c\nd\ne"},
		{"head exceeds", "a\nb", 5, 0, "a\nb"},
		{"tail exceeds", "a\nb", 5, 0, "a\nb"},
		{"tail with trailing newline", "a\nb\nc\n", 0, 2, "c\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LimitLines(tt.input, tt.head, tt.tail)
			if got != tt.expect {
				t.Errorf("LimitLines(%q, %d, %d) = %q, want %q", tt.input, tt.head, tt.tail, got, tt.expect)
			}
		})
	}
}
