package middleware

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzhttp"
)

// CompressOptions configures gzip response compression.
// CompressOptions 配置 gzip 响应压缩。
type CompressOptions struct {
	// Level accepts -2, -1, or 1 through 9. Zero uses gzip.DefaultCompression.
	// Level 接受 -2、-1 或 1 到 9；零值使用 gzip.DefaultCompression。
	Level int
	// MinSize is the minimum response size in bytes. Zero uses 1024.
	// MinSize 是开始压缩的最小响应字节数；零值使用 1024。
	MinSize int
	// ContentTypes restricts compression to these media types when non-empty.
	// Empty uses gzhttp's content type filter. Wildcards are not supported.
	// ContentTypes 非空时仅压缩这些媒体类型；为空时使用 gzhttp 的类型过滤器。
	// 不支持通配符。
	ContentTypes []string
}

// Compress negotiates gzip without buffering the entire response.
// It preserves flushing and hijacking, skips HEAD and already encoded responses,
// and removes ETag when compressing. Invalid options panic at construction.
// Compress 协商 gzip 压缩，不缓存完整响应。
// 保留刷新和连接接管能力，跳过 HEAD 和已编码响应，压缩时移除 ETag。
// 配置无效时在构造阶段 panic。
func Compress(opts CompressOptions) func(http.Handler) http.Handler {
	if opts.Level == 0 {
		opts.Level = gzip.DefaultCompression
	}
	if opts.Level < gzip.HuffmanOnly || opts.Level > gzip.BestCompression {
		panic("middleware: Compress Level must be between -2 and 9")
	}
	if opts.MinSize == 0 {
		opts.MinSize = 1024
	}
	contentTypes := gzhttp.ContentTypeFilter(gzhttp.DefaultContentTypeFilter)
	if len(opts.ContentTypes) > 0 {
		for _, contentType := range opts.ContentTypes {
			mediaType, _, err := mime.ParseMediaType(contentType)
			if err != nil || strings.Contains(mediaType, "*") {
				panic("middleware: Compress invalid content type: " + contentType)
			}
		}
		contentTypes = gzhttp.ContentTypes(opts.ContentTypes)
	}
	wrap, err := gzhttp.NewWrapper(
		gzhttp.CompressionLevel(opts.Level),
		gzhttp.MinSize(opts.MinSize),
		contentTypes,
		gzhttp.EnableZstd(false),
		gzhttp.DropETag(),
	)
	if err != nil {
		panic("middleware: Compress: " + err.Error())
	}
	return func(next http.Handler) http.Handler { return wrap(next) }
}
