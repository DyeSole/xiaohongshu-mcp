package xiaohongshu

import (
	"os"
	"strings"
)

// webURL 拼接网页版地址。XHS_WEB_BASE 可设为 https://www.rednote.com，
// 海外 IP 直接访问国际版，避免 xiaohongshu.com 跳转时丢失登录态。
func webURL(path string) string {
	base := strings.TrimRight(os.Getenv("XHS_WEB_BASE"), "/")
	if base == "" {
		base = "https://www.xiaohongshu.com"
	}
	return base + path
}
