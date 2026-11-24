// Package media 负责媒体源抽象、目录扫描与文件识别（3.1 / 9.1）。
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zys747526271/lanreel/server/internal/models"
)

// Item 一个媒体条目。
type Item struct {
	Key     string    `json:"key"`
	Source  string    `json:"source"`
	RelPath string    `json:"rel_path"`
	AbsPath string    `json:"-"`
	Type    string    `json:"media_type"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	// Duration 视频时长（秒），探测失败或图片为 0。
	Duration float64 `json:"duration"`
}

// Folder 返回条目所在文件夹的相对路径（统一使用 / 分隔，根目录为空串）。
func (i *Item) Folder() string {
	rel := NormalizeRel(i.RelPath)
	dir := path.Dir(rel)
	if dir == "." || dir == "/" {
		return ""
	}
	return dir
}

// NormalizeRel 将任意风格路径归一化为「无前导斜杠、/ 分隔」的相对路径。
func NormalizeRel(p string) string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	return path.Clean(p)
}

// NormalizeFolderPath 归一化文件夹相对路径：根目录统一为空串，其余统一为 / 分隔。
func NormalizeFolderPath(p string) string {
	p = NormalizeRel(p)
	if p == "." || p == "/" {
		return ""
	}
	return p
}

// MakeKey 计算 video_key：媒体源 ID + 根索引 + 源内相对路径的哈希（9.1）。
// 使用相对路径而非绝对路径，保证文件夹迁移后收藏关系不失效（F-19）。
func MakeKey(sourceID string, rootIndex int, relPath string) string {
	norm := NormalizeRel(relPath)
	h := sha256.New()
	h.Write([]byte(sourceID))
	h.Write([]byte{0})
	h.Write([]byte(strconv.Itoa(rootIndex)))
	h.Write([]byte{0})
	h.Write([]byte(norm))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// KindByExt 根据扩展名判断媒体类型，返回空串表示不支持。
func KindByExt(ext string, videoExts, imageExts []string) string {
	e := strings.ToLower(ext)
	for _, v := range videoExts {
		if strings.ToLower(v) == e {
			return models.MediaVideo
		}
	}
	for _, v := range imageExts {
		if strings.ToLower(v) == e {
			return models.MediaImage
		}
	}
	return ""
}
