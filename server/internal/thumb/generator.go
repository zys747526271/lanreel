// Package thumb 生成并缓存媒体缩略图（B-01）。
//
// 图片用标准库解码后缩放；视频依赖 ffmpeg 抽取一帧。两者统一输出
// {data_dir}/thumbs/{video_key}.jpg，先写临时文件再原子重命名，避免读到半个文件。
// ffmpeg 缺失时只记录一次日志并跳过视频缩略图，不影响扫描与播放。
package thumb

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zys747526271/lanreel/server/internal/logging"
	"github.com/zys747526271/lanreel/server/internal/media"
	"github.com/zys747526271/lanreel/server/internal/models"
)

// Config 缩略图参数。
type Config struct {
	// Dir 输出目录（通常为 {data_dir}/thumbs）。
	Dir string
	// FFmpegPath 显式指定的 ffmpeg 路径；空则自动探测。
	FFmpegPath string
	// Width 输出宽度（像素），高度按比例。
	Width int
	// SeekSeconds 视频抽帧位置（秒）。
	SeekSeconds float64
	// Quality JPEG 质量（1-100）。
	Quality int
	// Enabled 关闭后不生成，接口回落到原图/404。
	Enabled bool
}

// Stats 缩略图统计。
type Stats struct {
	Enabled         bool   `json:"enabled"`
	FFmpegAvailable bool   `json:"ffmpeg_available"`
	FFmpegPath      string `json:"ffmpeg_path,omitempty"`
	Generated       int    `json:"generated"`
	Failed          int    `json:"failed"`
	Queued          int    `json:"queued"`
}

// Generator 缩略图生成器（单 worker，避免与前台游戏争抢 CPU）。
type Generator struct {
	cfg Config
	log *logging.Logger

	queue  chan *media.Item
	ffmpeg string

	mu        sync.Mutex
	generated int
	failed    int
	warned    bool
	done      map[string]bool
}

// New 构造生成器并探测 ffmpeg。
func New(cfg Config, logger *logging.Logger) *Generator {
	if cfg.Width <= 0 {
		cfg.Width = 480
	}
	if cfg.Quality <= 0 {
		cfg.Quality = 78
	}
	if cfg.SeekSeconds <= 0 {
		cfg.SeekSeconds = 1
	}
	g := &Generator{
		cfg: cfg, log: logger,
		queue: make(chan *media.Item, 2048),
		done:  map[string]bool{},
	}
	g.ffmpeg = detectFFmpeg(cfg.FFmpegPath)
	return g
}

// detectFFmpeg 查找可用的 ffmpeg：显式配置 → PATH → 可执行文件同级目录。
func detectFFmpeg(configured string) string {
	if p := strings.TrimSpace(configured); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, name := range []string{"ffmpeg.exe", "ffmpeg"} {
			p := filepath.Join(dir, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

// Start 启动 worker；ctx 取消时退出。
func (g *Generator) Start(ctx context.Context) {
	if !g.cfg.Enabled {
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case item := <-g.queue:
				g.process(item, false)
			}
		}
	}()
	g.log.Info(logging.CatScan, "media.thumb", "缩略图服务已启动", map[string]any{
		"dir":              g.cfg.Dir,
		"ffmpeg_available": g.ffmpeg != "",
		"ffmpeg_path":      g.ffmpeg,
		"width":            g.cfg.Width,
	})
}

// Enqueue 把媒体加入生成队列（队列满时丢弃，下次扫描会重新入队）。
func (g *Generator) Enqueue(items []*media.Item) {
	if !g.cfg.Enabled {
		return
	}
	for _, it := range items {
		if it == nil {
			continue
		}
		if it.Type == models.MediaVideo && g.ffmpeg == "" {
			continue
		}
		select {
		case g.queue <- it:
		default:
			return
		}
	}
}

// Path 返回缩略图路径。
func (g *Generator) Path(key string) string {
	return filepath.Join(g.cfg.Dir, key+".jpg")
}

// Exists 缩略图是否已生成。
func (g *Generator) Exists(key string) bool {
	st, err := os.Stat(g.Path(key))
	return err == nil && !st.IsDir() && st.Size() > 0
}

// Stats 当前统计。
func (g *Generator) Stats() Stats {
	g.mu.Lock()
	defer g.mu.Unlock()
	return Stats{
		Enabled:         g.cfg.Enabled,
		FFmpegAvailable: g.ffmpeg != "",
		FFmpegPath:      g.ffmpeg,
		Generated:       g.generated,
		Failed:          g.failed,
		Queued:          len(g.queue),
	}
}

// Generate 同步生成单个缩略图（后台「重建」按钮使用）。
func (g *Generator) Generate(item *media.Item) error {
	return g.process(item, true)
}

// process 生成单个缩略图，force=true 时忽略已存在检查。
func (g *Generator) process(item *media.Item, force bool) error {
	if item == nil || !g.cfg.Enabled {
		return nil
	}
	out := g.Path(item.Key)
	if !force {
		if st, err := os.Stat(out); err == nil && !st.IsDir() && st.Size() > 0 {
			// 源文件未更新则直接复用。
			if !item.ModTime.After(st.ModTime()) {
				return nil
			}
		}
	}
	if err := os.MkdirAll(g.cfg.Dir, 0o755); err != nil {
		return err
	}
	var err error
	switch item.Type {
	case models.MediaImage:
		err = g.fromImage(item, out)
	case models.MediaVideo:
		if g.ffmpeg == "" {
			g.warnOnce()
			return nil
		}
		err = g.fromVideo(item, out)
	default:
		return nil
	}
	g.mu.Lock()
	if err != nil {
		g.failed++
	} else {
		g.generated++
	}
	g.mu.Unlock()
	if err != nil {
		g.log.Warn(logging.CatScan, "media.thumb", "缩略图生成失败", map[string]any{
			"video_key": item.Key, "rel_path": item.RelPath, "error": err.Error(),
		})
	}
	return err
}

func (g *Generator) warnOnce() {
	g.mu.Lock()
	warned := g.warned
	g.warned = true
	g.mu.Unlock()
	if !warned {
		g.log.Warn(logging.CatScan, "media.thumb",
			"未找到 ffmpeg，视频缩略图将跳过（可配置 media.ffmpeg_path 或安装 ffmpeg）", nil)
	}
}

// fromImage 解码图片并缩放为 JPEG。
func (g *Generator) fromImage(item *media.Item, out string) error {
	f, err := os.Open(item.AbsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("解码图片失败: %w", err)
	}
	w := g.cfg.Width
	b := src.Bounds()
	if b.Dx() <= w {
		w = b.Dx()
	}
	if w <= 0 {
		return errors.New("图片尺寸非法")
	}
	h := int(float64(b.Dy()) * float64(w) / float64(b.Dx()))
	if h <= 0 {
		h = 1
	}
	return writeJPEG(out, scaleBilinear(src, w, h), g.cfg.Quality)
}

// fromVideo 用 ffmpeg 抽取一帧并缩放。
func (g *Generator) fromVideo(item *media.Item, out string) error {
	tmp := out + ".tmp.jpg"
	_ = os.Remove(tmp)
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.2f", g.cfg.SeekSeconds),
		"-i", item.AbsPath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:-2", g.cfg.Width),
		"-q:v", "4",
		"-y", tmp,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, g.ffmpeg, args...)
	configureCmd(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		msg := strings.TrimSpace(string(out))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		if msg == "" {
			return err
		}
		return fmt.Errorf("%v: %s", err, msg)
	}
	if st, err := os.Stat(tmp); err != nil || st.Size() == 0 {
		_ = os.Remove(tmp)
		return errors.New("ffmpeg 未产出缩略图")
	}
	return os.Rename(tmp, out)
}

// scaleBilinear 双线性缩放（纯标准库实现，避免引入额外依赖）。
func scaleBilinear(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if sw <= 0 || sh <= 0 {
		return dst
	}
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		y0 := int(math.Floor(fy))
		ty := fy - float64(y0)
		if y0 < 0 {
			y0, ty = 0, 0
		}
		y1 := y0 + 1
		if y1 >= sh {
			y1 = sh - 1
		}
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			x0 := int(math.Floor(fx))
			tx := fx - float64(x0)
			if x0 < 0 {
				x0, tx = 0, 0
			}
			x1 := x0 + 1
			if x1 >= sw {
				x1 = sw - 1
			}
			r00, g00, b00, a00 := src.At(b.Min.X+x0, b.Min.Y+y0).RGBA()
			r10, g10, b10, a10 := src.At(b.Min.X+x1, b.Min.Y+y0).RGBA()
			r01, g01, b01, a01 := src.At(b.Min.X+x0, b.Min.Y+y1).RGBA()
			r11, g11, b11, a11 := src.At(b.Min.X+x1, b.Min.Y+y1).RGBA()
			blend := func(v00, v10, v01, v11 uint32) uint8 {
				top := float64(v00)*(1-tx) + float64(v10)*tx
				bottom := float64(v01)*(1-tx) + float64(v11)*tx
				v := top*(1-ty) + bottom*ty
				return uint8(math.Round(v / 257))
			}
			off := dst.PixOffset(x, y)
			dst.Pix[off] = blend(r00, r10, r01, r11)
			dst.Pix[off+1] = blend(g00, g10, g01, g11)
			dst.Pix[off+2] = blend(b00, b10, b01, b11)
			dst.Pix[off+3] = blend(a00, a10, a01, a11)
		}
	}
	return dst
}

// writeJPEG 原子写入 JPEG 文件。
func writeJPEG(path string, img image.Image, quality int) error {
	tmp := path + ".tmp.jpg"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if st, err := os.Stat(tmp); err != nil || st.Size() == 0 {
		_ = os.Remove(tmp)
		return errors.New("缩略图写入失败")
	}
	return os.Rename(tmp, path)
}
