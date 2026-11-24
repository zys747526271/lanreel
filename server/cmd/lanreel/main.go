// LanReel 服务端入口。
//
// 启动顺序：加载配置 → 初始化日志 → 打开业务库/日志库并迁移 → 初始化管理员
// → 扫描媒体 → 启动 HTTP 服务 → 后台日志清理。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/zys747526271/lanreel/server/internal/api"
	"github.com/zys747526271/lanreel/server/internal/auth"
	"github.com/zys747526271/lanreel/server/internal/config"
	"github.com/zys747526271/lanreel/server/internal/database"
	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/media"
	"github.com/zys747526271/lanreel/server/internal/policy"
	"github.com/zys747526271/lanreel/server/internal/repository"
	"github.com/zys747526271/lanreel/server/internal/service"
	"github.com/zys747526271/lanreel/server/internal/thumb"
)

// version 由构建脚本通过 -ldflags 注入。
var version = "0.1.0"

func main() {
	var (
		configPath   = flag.String("config", "", "配置文件路径（默认 ./config.yaml 或 LANREEL_CONFIG）")
		resetAdmin   = flag.Bool("reset-admin-password", false, "用启动配置中的初始密码重置管理员密码")
		scanOnly     = flag.Bool("scan-only", false, "仅执行一次媒体扫描并输出统计后退出")
		printVersion = flag.Bool("version", false, "输出版本号")
	)
	flag.Parse()

	if *printVersion {
		fmt.Println("lanreel", version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[lanreel] 配置加载失败: %v", err)
	}
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("[lanreel] 初始化目录失败: %v", err)
	}
	if err := cfg.LoadOrCreateSecret(func(n int) string {
		b := make([]byte, n)
		rand.Read(b)
		return hex.EncodeToString(b)
	}); err != nil {
		log.Fatalf("[lanreel] JWT 密钥初始化失败: %v", err)
	}

	// 业务库与日志库分别打开，连接与版本号互相独立（N-21 / N-24）。
	bizDB, err := database.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("[lanreel] 业务库打开失败: %v", err)
	}
	defer bizDB.Close()
	if err := database.MigrateBusiness(bizDB); err != nil {
		log.Fatalf("[lanreel] 业务库迁移失败: %v", err)
	}
	logDB, err := database.Open(cfg.Logging.Path)
	if err != nil {
		log.Fatalf("[lanreel] 日志库打开失败: %v", err)
	}
	defer logDB.Close()
	if err := database.MigrateLogs(logDB); err != nil {
		log.Fatalf("[lanreel] 日志库迁移失败: %v", err)
	}

	logStore := logging.NewSQLiteStore(logDB, cfg.Location)
	logger := logging.New(cfg.LogLevel(), cfg.Location, logStore, cfg.Logging.FallbackDir)
	logger.Start()
	defer logger.Close()

	store := repository.New(bizDB, cfg.Location)
	tokens := auth.NewManager(cfg.Auth.JWTSecret, cfg.AccessTTL(), cfg.RefreshTTL())

	authSvc := service.NewAuthService(store, tokens, logger, cfg)
	if err := authSvc.InitializeAdmin(context.Background(), *resetAdmin); err != nil {
		logger.Error(logging.CatSystem, "app.bootstrap", "管理员账号初始化失败", map[string]any{"error": err})
		if *scanOnly {
			return
		}
		log.Fatalf("[lanreel] 管理员初始化失败: %v", err)
	}
	if !*scanOnly && authSvc.SetupRequired(context.Background()) {
		fmt.Printf("尚未创建管理员：请打开 http://<本机或局域网IP>:%d 按页面引导完成初始化\n", cfg.Server.Port)
	}

	dataDir := cfg.App.DataDir
	if dataDir == "" {
		dataDir = "./data"
	}
	thumbGen := thumb.New(thumb.Config{
		Dir:         filepath.Join(dataDir, "thumbs"),
		FFmpegPath:  cfg.Media.FFmpegPath,
		Width:       cfg.Media.ThumbnailWidth,
		SeekSeconds: cfg.Media.ThumbnailSeekSeconds,
		Enabled:     cfg.Media.ThumbnailEnabled,
	}, logger)

	scanner := media.NewScanner(cfg, store, logger)
	scanner.SetOnScan(thumbGen.Enqueue)
	evaluator := policy.NewEvaluator(store, cfg.Location, logger)
	mediaSvc := service.NewMediaService(scanner, evaluator, cfg, logger)
	favSvc := service.NewFavoriteService(store, mediaSvc, logger)
	adminSvc := service.NewAdminService(store, mediaSvc, favSvc, evaluator, authSvc, cfg, logger, logStore)
	// 后台可覆盖媒体目录（CR-003）：启动时优先使用已保存的设置。
	scanner.SetRoots(adminSvc.EffectiveMediaRoots(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *scanOnly {
		snap, summary, err := scanner.Scan(ctx)
		if err != nil {
			log.Fatalf("[lanreel] 扫描失败: %v", err)
		}
		fmt.Printf("扫描完成：共 %d 项（视频 %d / 图片 %d），跳过 %d，新增 %d，移除 %d，耗时 %dms，快照 %d 项\n",
			summary.Total, summary.Videos, summary.Images, summary.Skipped, summary.Added,
			summary.Removed, summary.DurationMS, len(snap.Items))
		return
	}

	// 启动日志（配置摘要与资源限制参数，10.2）。
	logger.Info(logging.CatSystem, "app.bootstrap", "LanReel 服务启动", map[string]any{
		"version":       version,
		"addr":          cfg.Addr(),
		"tls":           cfg.Server.TLS.Enabled,
		"media_roots":   len(cfg.Media.Roots),
		"scan_interval": cfg.Media.ScanIntervalSeconds,
		"timezone":      cfg.App.Timezone,
		"db":            cfg.Database.Path,
		"log_db":        cfg.Logging.Path,
		"log_level":     cfg.LogLevel(),
		"gomemlimit":    os.Getenv("GOMEMLIMIT"),
		"gomaxprocs":    os.Getenv("GOMAXPROCS"),
		"config_file":   cfg.ConfigFile,
	})

	thumbGen.Start(ctx)
	scanner.Start(ctx)
	if cfg.Media.WatchEnabled {
		watcher := media.NewWatcher(scanner, logger,
			time.Duration(cfg.Media.WatchDebounceSeconds)*time.Second, cfg.Media.WatchMaxDirs)
		go watcher.Run(ctx)
	}

	handlers := api.NewHandlers(cfg, authSvc, mediaSvc, favSvc, adminSvc, scanner, store, logger, logStore, thumbGen, version)
	engine := handlers.Engine()

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go startLogMaintenance(ctx, store, logStore, logger, cfg)
	// 登录限流桶定期清理，避免长期运行内存增长（B-05）。
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				authSvc.Limiter().Cleanup()
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		if cfg.Server.TLS.Enabled {
			if cfg.Server.TLS.CertFile == "" || cfg.Server.TLS.KeyFile == "" {
				errCh <- errors.New("启用 TLS 但未配置 cert_file / key_file")
				return
			}
			logger.Info(logging.CatSystem, "app.http", "以 HTTPS 启动", map[string]any{"addr": cfg.Addr()})
			errCh <- srv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
			return
		}
		logger.Info(logging.CatSystem, "app.http", "以 HTTP 启动", map[string]any{"addr": cfg.Addr()})
		errCh <- srv.ListenAndServe()
	}()

	fmt.Printf("LanReel %s 已启动，监听 %s（媒体根目录：%v）\n", version, cfg.Addr(), cfg.Media.Roots)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-quit:
		logger.Info(logging.CatSystem, "app.http", "收到退出信号，开始优雅关闭", map[string]any{"signal": sig.String()})
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(logging.CatError, "app.http", "HTTP 服务异常退出", map[string]any{"error": err})
			log.Fatalf("[lanreel] 服务异常退出: %v", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn(logging.CatSystem, "app.http", "关闭 HTTP 服务时出错", map[string]any{"error": err})
	}
	logger.Info(logging.CatSystem, "app.bootstrap", "LanReel 服务已停止", nil)
	logger.Close()
}

// startLogMaintenance 启动时与每日低峰执行日志清理（F-40 / N-28）。
func startLogMaintenance(ctx context.Context, store *repository.Store, logStore *logging.SQLiteStore,
	logger *logging.Logger, cfg *config.Config) {
	run := func() {
		retention := cfg.Logging.RetentionDays
		maxEntries := cfg.Logging.MaxEntries
		if v, err := strconv.Atoi(store.GetSetting(ctx, service.SettingLogRetentionDays, strconv.Itoa(retention))); err == nil && v > 0 {
			retention = v
		}
		if v, err := strconv.Atoi(store.GetSetting(ctx, service.SettingLogMaxEntries, strconv.Itoa(maxEntries))); err == nil && v > 0 {
			maxEntries = v
		}
		before := time.Now().AddDate(0, 0, -retention)
		n, err := logStore.Purge(ctx, before, maxEntries)
		if err != nil {
			logger.Warn(logging.CatSystem, "app.logs", "日志清理失败", map[string]any{"error": err})
			return
		}
		// 清理动作本身写入 system 分类（N-28）。
		logger.Info(logging.CatSystem, "app.logs", "运行日志清理完成", map[string]any{
			"removed": n, "retention_days": retention, "max_entries": maxEntries,
		})
	}

	run()
	for {
		now := time.Now().In(cfg.Location)
		next := time.Date(now.Year(), now.Month(), now.Day(), 3, 30, 0, 0, cfg.Location)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
			run()
		}
	}
}
