package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"testing"
)

func TestExtractRejectsUnsafeEntries(t *testing.T) {
	for _, h := range []*tar.Header{{Name: "../escape", Typeflag: tar.TypeReg}, {Name: "/absolute", Typeflag: tar.TypeReg}, {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../escape"}, {Name: "large", Typeflag: tar.TypeReg, Size: 513 << 20}} {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		tr := tar.NewWriter(gz)
		tr.WriteHeader(h)
		tr.Close()
		gz.Close()
		if e := extract(context.Background(), b.Bytes(), t.TempDir()); e == nil {
			t.Fatal("accepted", h)
		}
	}
}
