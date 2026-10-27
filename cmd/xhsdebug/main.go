package main

import (
	"fmt"
	"os"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/xpzouying/xiaohongshu-mcp/browser"
	"github.com/xpzouying/xiaohongshu-mcp/configs"
)

func main() {
	b := browser.NewBrowser(true, browser.WithFingerprintSeed(configs.FingerprintSeedFromEnv()), browser.WithProxy(configs.ProxyFromEnv()))
	defer b.Close()
	page := b.NewPage().Timeout(120 * time.Second)
	if r, err := page.Eval(`() => JSON.stringify({ua: navigator.userAgent, webdriver: navigator.webdriver, platform: navigator.platform, langs: navigator.languages, tz: Intl.DateTimeFormat().resolvedOptions().timeZone, plugins: navigator.plugins.length, gpu: (()=>{try{const c=document.createElement('canvas').getContext('webgl');const d=c.getExtension('WEBGL_debug_renderer_info');return c.getParameter(d.UNMASKED_RENDERER_WEBGL)}catch(e){return 'none'}})()})`); err == nil {
		fmt.Println("env:", r.Value.String())
	}
	page.MustNavigate(os.Args[1])
	time.Sleep(12 * time.Second)
	fmt.Println("url:", page.MustInfo().URL, "title:", page.MustInfo().Title)
	for _, sel := range []string{".login-container", ".login-container .qrcode-img", ".qrcode-img", ".main-container .user .link-wrapper .channel", ".login-btn", "#captcha", ".wendian-btn"} {
		has, _, _ := page.Has(sel)
		fmt.Println(sel, has)
	}
	img, _ := page.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	_ = os.WriteFile(os.Args[2], img, 0o644)
}
