package xiaohongshu

import (
	"errors"
	"testing"
)

func TestNormalizeNoteURL(t *testing.T) {
	t.Setenv("XHS_WEB_BASE", "")
	const want = "https://www.xiaohongshu.com/explore/6ad03dd8000000001801a832?xsec_token=CBZ%3D&xsec_source=pc_feed"
	resolve := func(link string) (string, error) {
		if link != "http://xhslink.com/o/abc" && link != "https://xhslink.cn/o/abc" {
			return "", errors.New("unexpected " + link)
		}
		return "https://www.xiaohongshu.com/discovery/item/6ad03dd8000000001801a832?app_platform=ios&xsec_token=CBZ=&type=video", nil
	}
	noResolve := func(string) (string, error) { return "", errors.New("should not resolve") }

	cases := []struct {
		name, raw, id, token string
		resolver             func(string) (string, error)
	}{
		{"short link", "http://xhslink.com/o/abc", "", "", resolve},
		{"app share text", "萌死了 http://xhslink.com/o/abc 复制后打开【小红书】查看笔记！", "", "", resolve},
		{"xhslink.cn share text", "谁说宗门里个个仙风道骨的呢 https://xhslink.cn/o/abc 复制文字后，打开【小红书】就能查看笔记哦！", "", "", resolve},
		{"full link", "https://www.xiaohongshu.com/explore/6ad03dd8000000001801a832?xsec_token=CBZ=&xsec_source=pc_user", "", "", noResolve},
		{"rednote link", "https://www.rednote.com/discovery/item/6ad03dd8000000001801a832?xsec_token=CBZ=", "", "", noResolve},
		{"id and token", "", "6ad03dd8000000001801a832", "CBZ=", noResolve},
	}
	for _, tc := range cases {
		got, err := NormalizeNoteURL(tc.raw, tc.id, tc.token, tc.resolver)
		if err != nil || got != want {
			t.Errorf("%s: got %q err %v", tc.name, got, err)
		}
	}

	for _, bad := range []string{"", "https://example.com/explore/6ad03dd8000000001801a832?xsec_token=x",
		"https://www.xiaohongshu.com/explore/6ad03dd8000000001801a832"} {
		if _, err := NormalizeNoteURL(bad, "", "", noResolve); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
