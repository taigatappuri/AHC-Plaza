package params

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplacePreservesSource(t *testing.T) {
	s := []byte("// 50 stays\r\nconstexpr int WIDTH = 50; // @tune 10 200\r\nstatic float TEMP = 1.0f; // @tune 0.5 4 log\r\nconst char* text = R\"tag(// @tune 0 10 { })tag\";\r\nint a[WIDTH];\r\nint main(){return a[0];}\r\n")
	scan, e := Parse(s)
	if e != nil {
		t.Fatal(e)
	}
	if len(scan.Parameters) != 2 {
		t.Fatal(scan)
	}
	out, actual, e := Generate(s, scan.Hash, scan.Parameters, map[string]float64{"WIDTH": 25, "TEMP": 0.7})
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(out), "int WIDTH = 25;") || !strings.Contains(string(out), "// 50 stays\r\n") || actual["TEMP"] != float64(float32(0.7)) {
		t.Fatal(string(out), actual)
	}
	compiler, e := exec.LookPath("g++")
	if e != nil {
		t.Skip("g++ unavailable")
	}
	path := filepath.Join(t.TempDir(), "main.cpp")
	os.WriteFile(path, out, 0600)
	if b, e := exec.Command(compiler, "-std=c++17", "-fsyntax-only", path).CombinedOutput(); e != nil {
		t.Fatalf("%s: %v", b, e)
	}
}
func TestRejectUnsupported(t *testing.T) {
	for _, s := range []string{
		"void f(){int X=1; // @tune 1 4\n}", "#if 1\nint X=1; // @tune 1 4\n#endif", "int X=1<<2; // @tune 1 4", "auto X=1; // @tune 1 4", "int X=1; // @tune 1 4\nint X=2; // @tune 1 4", "int X=1; // @tune 0 4 log", "double X=1; // @tune 0 1 step=0.3", "long long X=9007199254740992LL; // @tune 1 2", "int X=1; /* @tune 1 2 */",
	} {
		t.Run(s, func(t *testing.T) {
			if _, e := Parse([]byte(s)); e == nil {
				t.Fatal("accepted", s)
			}
		})
	}
}
func TestRangeAndHash(t *testing.T) {
	s := []byte("double X=-1e-3; // @tune 0 1 step=0.05\nint Y=1; // @tune\n")
	scan, e := Parse(s)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Resolve(scan, nil); e == nil {
		t.Fatal("missing bounds accepted")
	}
	if _, _, e = Generate(s, "wrong", scan.Parameters, nil); e == nil {
		t.Fatal("hash accepted")
	}
}

func TestUnrelatedDigitSeparatorsAndContinuedComments(t *testing.T) {
	source := "long long INF=1'000'000;\nint X=1; // @tune 1 3\n"
	scan, e := Parse([]byte(source))
	if e != nil || len(scan.Parameters) != 1 {
		t.Fatal(scan, e)
	}
	source = "// ignored \\\nint X=1; // @tune 1 3\n"
	if _, e = Parse([]byte(source)); e == nil {
		t.Fatal("continued comment accepted")
	}
}
