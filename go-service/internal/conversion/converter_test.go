package conversion

import "testing"

func TestSelectableTemplates(t *testing.T) {
	want := []string{"competition", "hyperion", "icarus", "management", "safety", "space-race"}
	got := SelectableTemplates()
	if len(got) != len(want) {
		t.Fatalf("SelectableTemplates() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SelectableTemplates() = %v, want %v", got, want)
		}
	}
}

func TestRepairInvalidStrikeoutFallback(t *testing.T) {
	invalid := `\IfFileExists{soul.sty}{\providecommand{\sout}[1]{\st{#1}}}{\providecommand{\sout}[1]{#1}}`
	want := `\IfFileExists{soul.sty}{\providecommand{\sout}[1]{\st{##1}}}{\providecommand{\sout}[1]{##1}}`

	if got := repairInvalidStrikeoutFallback(invalid); got != want {
		t.Fatalf("repairInvalidStrikeoutFallback() = %q, want %q", got, want)
	}
}
