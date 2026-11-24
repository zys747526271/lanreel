package media

import (
	"encoding/binary"
	"io"
	"os"
)

// ProbeDuration 读取 MP4/MOV 容器的时长（秒）。
// 仅解析 moov/mvhd，不依赖 FFmpeg，避免引入转码依赖（N-04）；
// 解析失败（含 WebM）返回 0，由前端 <video> 的 loadedmetadata 兜底。
func ProbeDuration(filePath string) float64 {
	f, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return 0
	}
	size := st.Size()
	var offset int64
	// 顶层 atom 遍历，最多 64 次防止异常文件死循环。
	for i := 0; i < 64 && offset < size; i++ {
		boxSize, boxType, payload, hdr, err := readBoxHeader(f, offset)
		if err != nil {
			return 0
		}
		if boxSize <= 0 {
			return 0
		}
		if boxType == "moov" {
			return readMvhd(f, payload, payload+minInt64(boxSize-hdr, size-payload))
		}
		offset += boxSize
	}
	return 0
}

func readBoxHeader(f *os.File, offset int64) (boxSize int64, boxType string, payload int64, hdr int64, err error) {
	var buf [16]byte
	if _, err = f.ReadAt(buf[:8], offset); err != nil {
		return
	}
	boxSize = int64(binary.BigEndian.Uint32(buf[0:4]))
	boxType = string(buf[4:8])
	hdr = 8
	if boxSize == 1 {
		if _, err = f.ReadAt(buf[8:16], offset+8); err != nil {
			return
		}
		boxSize = int64(binary.BigEndian.Uint64(buf[8:16]))
		hdr = 16
	} else if boxSize == 0 {
		// 延伸到文件末尾
		st, serr := f.Stat()
		if serr != nil {
			err = serr
			return
		}
		boxSize = st.Size() - offset
	}
	payload = offset + hdr
	if boxSize < hdr {
		err = io.ErrUnexpectedEOF
	}
	return
}

func readMvhd(f *os.File, start, end int64) float64 {
	var offset int64 = start
	for i := 0; i < 32 && offset < end; i++ {
		boxSize, boxType, payload, hdr, err := readBoxHeader(f, offset)
		if err != nil || boxSize <= 0 {
			return 0
		}
		if boxType == "mvhd" {
			data := make([]byte, minInt64(boxSize-hdr, 64))
			if _, err := f.ReadAt(data, payload); err != nil && len(data) == 0 {
				return 0
			}
			if len(data) < 20 {
				return 0
			}
			version := data[0]
			if version == 1 {
				// version(1)+flags(3)+creation(8)+modification(8)+timescale(4)+duration(8)
				if len(data) < 32 {
					return 0
				}
				timescale := binary.BigEndian.Uint32(data[20:24])
				duration := binary.BigEndian.Uint64(data[24:32])
				if timescale == 0 {
					return 0
				}
				return float64(duration) / float64(timescale)
			}
			// version(1)+flags(3)+creation(4)+modification(4)+timescale(4)+duration(4)
			if len(data) < 20 {
				return 0
			}
			timescale := binary.BigEndian.Uint32(data[12:16])
			duration := binary.BigEndian.Uint32(data[16:20])
			if timescale == 0 {
				return 0
			}
			return float64(duration) / float64(timescale)
		}
		offset += boxSize
	}
	return 0
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
