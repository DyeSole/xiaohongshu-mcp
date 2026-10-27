package xiaohongshu

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	linkInTextPattern = regexp.MustCompile(`https?://[^\s，。！？"'<>]+`)
	notePathPattern   = regexp.MustCompile(`/(?:explore|discovery/item)/([0-9a-fA-F]{24})`)
)

// NormalizeNoteURL 把短链、App 分享文案、完整链接或 笔记ID+xsec_token 统一成点点认得的完整笔记链接。
// 设计是：点点不认 xhslink 短链，只认带笔记 ID 和 xsec_token 的完整链接。
func NormalizeNoteURL(raw, feedID, xsecToken string, resolve func(string) (string, error)) (string, error) {
	if feedID != "" && xsecToken != "" {
		return buildNoteURL(feedID, xsecToken), nil
	}
	link := linkInTextPattern.FindString(raw)
	if link == "" {
		return "", fmt.Errorf("没有找到小红书链接")
	}
	for hop := 0; hop < 3; hop++ {
		parsed, err := url.Parse(link)
		if err != nil {
			return "", fmt.Errorf("链接格式错误: %w", err)
		}
		host := strings.ToLower(parsed.Hostname())
		switch {
		case host == "xhslink.com" || strings.HasSuffix(host, ".xhslink.com"):
			if link, err = resolve(link); err != nil {
				return "", fmt.Errorf("解析短链失败: %w", err)
			}
			continue
		case strings.HasSuffix(host, "xiaohongshu.com") || strings.HasSuffix(host, "rednote.com"):
			match := notePathPattern.FindStringSubmatch(parsed.Path)
			token := parsed.Query().Get("xsec_token")
			if match == nil || token == "" {
				return "", fmt.Errorf("链接里缺少笔记 ID 或 xsec_token")
			}
			return buildNoteURL(match[1], token), nil
		default:
			return "", fmt.Errorf("不是小红书链接: %s", host)
		}
	}
	return "", fmt.Errorf("短链跳转次数过多")
}

func buildNoteURL(feedID, xsecToken string) string {
	return webURL(fmt.Sprintf("/explore/%s?xsec_token=%s&xsec_source=pc_feed", feedID, url.QueryEscape(xsecToken)))
}

// ResolveShortLink 只读取短链的跳转地址，不跟随跳转、不开浏览器。
func ResolveShortLink(link string) (string, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(link)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("短链没有返回跳转地址（HTTP %d）", resp.StatusCode)
	}
	return location, nil
}
