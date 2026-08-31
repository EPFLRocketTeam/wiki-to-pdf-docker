package conversion

import "testing"

func TestRepairInvalidStrikeoutFallback(t *testing.T) {
	invalid := `\IfFileExists{soul.sty}{\providecommand{\sout}[1]{\st{#1}}}{\providecommand{\sout}[1]{#1}}`
	want := `\IfFileExists{soul.sty}{\providecommand{\sout}[1]{\st{##1}}}{\providecommand{\sout}[1]{##1}}`

	if got := repairInvalidStrikeoutFallback(invalid); got != want {
		t.Fatalf("repairInvalidStrikeoutFallback() = %q, want %q", got, want)
	}
}