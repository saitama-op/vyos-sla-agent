package probe

import "testing"

func TestNormalizeICMPSequence(t *testing.T) {
	tests := []struct {
		name string
		seq  int
		want int
	}{
		{name: "below boundary", seq: 65535, want: 65535},
		{name: "wraps at boundary", seq: 65536, want: 0},
		{name: "continues after wrap", seq: 65537, want: 1},
		{name: "multiple wraps", seq: 131073, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeICMPSequence(tt.seq); got != tt.want {
				t.Errorf("normalizeICMPSequence(%d) = %d, want %d", tt.seq, got, tt.want)
			}
		})
	}
}
