package media

import "testing"

func TestMakeKeyStableAndScoped(t *testing.T) {
	a := MakeKey("local", 0, "sub/a.mp4")
	b := MakeKey("local", 0, "sub/a.mp4")
	if a != b {
		t.Fatalf("同一输入必须得到相同 key: %s != %s", a, b)
	}
	if len(a) != 32 {
		t.Fatalf("key 长度应为 32，实际 %d", len(a))
	}
	if MakeKey("local", 1, "sub/a.mp4") == a {
		t.Fatal("不同根索引应产生不同 key，避免多根目录冲突")
	}
	if MakeKey("smb", 0, "sub/a.mp4") == a {
		t.Fatal("不同媒体源应产生不同 key")
	}
	if MakeKey("local", 0, "sub/b.mp4") == a {
		t.Fatal("不同相对路径应产生不同 key")
	}
}

func TestMakeKeyIgnoresPathStyle(t *testing.T) {
	// 相对路径统一为 / 分隔且去掉前导 ./，保证 Windows 与 Linux 扫描结果一致（F-19）。
	a := MakeKey("local", 0, "sub/a.mp4")
	b := MakeKey("local", 0, "./sub//a.mp4")
	if a != b {
		t.Fatalf("路径风格差异不应影响 key: %s != %s", a, b)
	}
}

func TestItemFolder(t *testing.T) {
	cases := []struct {
		rel  string
		want string
	}{
		{"a.mp4", ""},
		{"sub/a.mp4", "sub"},
		{"sub/deep/a.mp4", "sub/deep"},
		{"./sub/a.mp4", "sub"},
	}
	for _, c := range cases {
		it := &Item{RelPath: c.rel}
		if got := it.Folder(); got != c.want {
			t.Errorf("Folder(%q) = %q, want %q", c.rel, got, c.want)
		}
	}
}

func TestKindByExt(t *testing.T) {
	videos := []string{".mp4", ".webm"}
	images := []string{".jpg", ".png"}
	if got := KindByExt(".MP4", videos, images); got != "video" {
		t.Errorf("扩展名应大小写不敏感，得到 %q", got)
	}
	if got := KindByExt(".png", videos, images); got != "image" {
		t.Errorf("png 应为图片，得到 %q", got)
	}
	if got := KindByExt(".txt", videos, images); got != "" {
		t.Errorf("不支持的格式应返回空串，得到 %q", got)
	}
}

func TestNormalizeFolderPath(t *testing.T) {
	if got := NormalizeFolderPath(""); got != "" {
		t.Errorf("空路径应归一化为根目录，得到 %q", got)
	}
	if got := NormalizeFolderPath("."); got != "" {
		t.Errorf("点路径应归一化为根目录，得到 %q", got)
	}
	if got := NormalizeFolderPath("/sub/"); got != "sub" {
		t.Errorf("应去掉首尾斜杠，得到 %q", got)
	}
}
