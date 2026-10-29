// Package thumbs 为图片和视频生成缩略图。
// 图片（JPEG/PNG/GIF）用标准库解码 + x/image 高质量缩放；
// 视频用 ffmpeg 抽帧后走同一缩放管线。结果按 (路径, 大小, mtime) 缓存于内存。
package thumbs

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif" // 注册 GIF 解码器（image.Decode 使用）
	"image/jpeg"
	_ "image/png" // 注册 PNG 解码器
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/image/draw"
)

// DefaultSize 是缩略图的最大边长（像素）。
const DefaultSize = 256

const ffmpegTimeout = 20 * time.Second

// Result 是一次缩略图生成的输出（统一为 JPEG）。
type Result struct {
	Data        []byte
	ContentType string
}

var (
	mu    sync.Mutex
	cache = map[string]Result{}
)

// Generate 为 absPath 生成缩略图。kind 取 "image" 或 "video"。
func Generate(absPath, kind string, maxDim int) (Result, error) {
	if maxDim <= 0 {
		maxDim = DefaultSize
	}
	st, err := os.Stat(absPath)
	if err != nil {
		return Result{}, err
	}
	if st.IsDir() {
		return Result{}, errors.New("is a directory")
	}
	key := filepath.Clean(absPath) + "|" +
		strconv.FormatInt(st.Size(), 10) + "|" +
		strconv.FormatInt(st.ModTime().UnixNano(), 10) + "|" +
		strconv.Itoa(maxDim)

	mu.Lock()
	if r, ok := cache[key]; ok {
		mu.Unlock()
		return r, nil
	}
	mu.Unlock()

	img, err := decode(absPath, kind)
	if err != nil {
		return Result{}, err
	}
	r, err := scale(img, maxDim)
	if err != nil {
		return Result{}, err
	}

	mu.Lock()
	cache[key] = r
	mu.Unlock()
	return r, nil
}

func decode(absPath, kind string) (image.Image, error) {
	if kind == "video" {
		return ffmpegFrame(absPath, true)
	}
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if img, _, err := image.Decode(f); err == nil {
		return img, nil
	}
	// WebP/BMP/HEIC/TIFF 等标准库不支持的格式，回退 ffmpeg 抽帧
	return ffmpegFrame(absPath, false)
}

// ffmpegFrame 用 ffmpeg 抽一帧，先缩到 640 宽以内以加速后续缩放。视频取第 1 秒附近。
func ffmpegFrame(absPath string, isVideo bool) (image.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()
	args := []string{"-loglevel", "error"}
	if isVideo {
		args = append(args, "-ss", "1")
	}
	args = append(args, "-i", absPath,
		"-frames:v", "1",
		"-vf", "scale='min(640,iw)':-2",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-",
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // 避免每次抽帧弹出黑色控制台窗口
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("ffmpeg timeout")
		}
		return nil, errors.New("ffmpeg: " + strings.TrimSpace(errBuf.String()))
	}
	return jpeg.Decode(&out)
}

// TranscodeToJPEG 把 HEIC/HEIF 等原始字节转成 JPEG（上传时用，浏览器才能预览）。
func TranscodeToJPEG(src []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-frames:v", "1",
		"-q:v", "3",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // 避免 HEIC 转码时弹出黑色控制台窗口
	cmd.Stdin = bytes.NewReader(src)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("ffmpeg timeout")
		}
		return nil, errors.New("ffmpeg: " + strings.TrimSpace(errBuf.String()))
	}
	if out.Len() == 0 {
		return nil, errors.New("empty transcode output")
	}
	return out.Bytes(), nil
}

func scale(src image.Image, maxDim int) (Result, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return Result{}, errors.New("empty image")
	}
	nw, nh := w, h
	if w >= h {
		if w > maxDim {
			nw = maxDim
			nh = h * maxDim / w
		}
	} else {
		if h > maxDim {
			nh = maxDim
			nw = w * maxDim / h
		}
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return Result{}, err
	}
	return Result{Data: buf.Bytes(), ContentType: "image/jpeg"}, nil
}
