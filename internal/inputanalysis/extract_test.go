package inputanalysis

import (
	"strings"
	"testing"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
)

func TestExtractReadsConfiguredNumericPositions(t *testing.T) {
	variables := []config.InputVariableConfig{
		{Name: "N", Line: 1, Column: 1},
		{Name: "M", Line: 1, Column: 3},
		{Name: "X", Line: 3, Column: 2},
	}
	result, err := Extract(strings.NewReader("10   ignored 2.5e1\n\n-3 4.5\n"), variables)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]float64{"N": 10, "M": 25, "X": 4.5} {
		if got := result.Values[name]; got != want {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestExtractReportsMissingAndNonNumericValuesSeparately(t *testing.T) {
	variables := []config.InputVariableConfig{
		{Name: "N", Line: 1, Column: 1},
		{Name: "M", Line: 1, Column: 2},
		{Name: "K", Line: 3, Column: 1},
	}
	result, err := Extract(strings.NewReader("12 nope\n"), variables)
	if err != nil {
		t.Fatal(err)
	}
	if result.Values["N"] != 12 {
		t.Fatalf("N = %v, want 12", result.Values["N"])
	}
	if result.Errors["M"] == "" || result.Errors["K"] == "" {
		t.Fatalf("errors = %#v", result.Errors)
	}
}

func TestExtractRejectsNonFiniteValues(t *testing.T) {
	variables := []config.InputVariableConfig{{Name: "N", Line: 1, Column: 1}}
	result, err := Extract(strings.NewReader("NaN\n"), variables)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Values["N"]; ok || result.Errors["N"] == "" {
		t.Fatalf("result = %#v", result)
	}
}
