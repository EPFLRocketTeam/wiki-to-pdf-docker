package conversion

import "testing"

func TestRepairLegacyTemplateMacros(t *testing.T) {
	legacy := `\providecommand{\sout}[1]{\st{##1}}
\providecommand{\sout}[1]{##1}`
	want := `\providecommand{\sout}[1]{\st{#1}}
\providecommand{\sout}[1]{#1}`

	if got := repairLegacyTemplateMacros(legacy); got != want {
		t.Fatalf("repairLegacyTemplateMacros() = %q, want %q", got, want)
	}
}