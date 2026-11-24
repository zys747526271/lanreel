package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.Server.Port != 8080 {
		t.Errorf("默认端口应为 8080，实际 %d", cfg.Server.Port)
	}
	if len(cfg.Media.Roots) == 0 {
		t.Fatal("默认应包含媒体根目录占位")
	}
	if cfg.Logging.Path == cfg.Database.Path {
		t.Fatal("日志库必须与业务库分离（N-24）")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
server:
  host: 127.0.0.1
  port: 9090
media:
  roots:
    - "` + filepath.ToSlash(dir) + `/media"
app:
  timezone: "Asia/Shanghai"
auth:
  username: "root"
app2: {}
`
	// 上面故意包含未知键 app2，验证 YAML 解析不会因多余字段失败。
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LANREEL_PORT", "19090")
	t.Setenv("ADMIN_INIT_PASSWORD", "Secret@123")
	t.Setenv("LANREEL_LOG_LEVEL", "warn")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.Server.Port != 19090 {
		t.Errorf("环境变量应覆盖端口，实际 %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("配置文件中的 host 应生效，实际 %q", cfg.Server.Host)
	}
	if cfg.Auth.InitPassword != "Secret@123" {
		t.Error("ADMIN_INIT_PASSWORD 应映射到管理员初始密码")
	}
	if cfg.LogLevel() != "warn" {
		t.Errorf("日志级别应为 warn，实际 %q", cfg.LogLevel())
	}
	if cfg.Location == nil || cfg.Location.String() != "Asia/Shanghai" {
		t.Errorf("时区应解析为 Asia/Shanghai，实际 %v", cfg.Location)
	}
}

func TestInvalidPort(t *testing.T) {
	t.Setenv("LANREEL_PORT", "70000")
	if _, err := Load(""); err == nil {
		t.Fatal("非法端口应返回错误")
	}
}

func TestSplitList(t *testing.T) {
	got := splitList("D:/a;D:/b,D:/c")
	if len(got) != 3 {
		t.Fatalf("应解析出 3 个目录，实际 %v", got)
	}
}
