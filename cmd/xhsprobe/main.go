// xhsprobe 调试用：带登录态打开页面，按步骤点击/输入并导出页面结构和截图。
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/xpzouying/xiaohongshu-mcp/browser"
	"github.com/xpzouying/xiaohongshu-mcp/configs"
)

const dumpJS = `(pattern) => {
	const re = new RegExp(pattern, 'i');
	const out = [];
	for (const el of document.querySelectorAll('body *')) {
		const own = [...el.childNodes].filter(n => n.nodeType === 3).map(n => n.textContent.trim()).join('');
		const cls = typeof el.className === 'string' ? el.className : '';
		const attrs = ['placeholder','contenteditable','role','data-placeholder'].map(a => el.getAttribute(a) ? a+'='+el.getAttribute(a) : '').filter(Boolean).join(' ');
		if (re.test(own + ' ' + cls + ' ' + attrs) && own.length < 80) {
			const r = el.getBoundingClientRect();
			out.push(el.tagName + '.' + cls.slice(0, 70) + ' | ' + own + ' | ' + attrs + ' | ' + Math.round(r.x) + ',' + Math.round(r.y) + ' ' + Math.round(r.width) + 'x' + Math.round(r.height));
		}
	}
	return out.slice(0, 60).join('\n');
}`

func main() {
	// 用法: xhsprobe URL OUTDIR [步骤...]  步骤: wait:秒 | click:文字 | clicksel:CSS | type:CSS=>文字 | enter | dump:正则 | shot:名字 | text:CSS
	b := browser.NewBrowser(true, browser.WithFingerprintSeed(configs.FingerprintSeedFromEnv()))
	defer b.Close()
	page := b.NewPage().Timeout(10 * time.Minute)
	page.MustNavigate(os.Args[1])
	out := os.Args[2]
	for _, step := range os.Args[3:] {
		kind, arg, _ := strings.Cut(step, ":")
		switch kind {
		case "wait":
			var s int
			fmt.Sscan(arg, &s)
			time.Sleep(time.Duration(s) * time.Second)
		case "click":
			el, err := page.Timeout(15*time.Second).ElementR("*", "^"+arg+"$")
			if err != nil {
				fmt.Println("click 找不到:", arg, err)
				continue
			}
			el.CancelTimeout().MustClick()
		case "clicksel":
			el, err := page.Timeout(15 * time.Second).Element(arg)
			if err != nil {
				fmt.Println("clicksel 找不到:", arg, err)
				continue
			}
			el.CancelTimeout().MustClick()
		case "type":
			sel, text, _ := strings.Cut(arg, "=>")
			el, err := page.Timeout(15 * time.Second).Element(sel)
			if err != nil {
				fmt.Println("type 找不到:", sel, err)
				continue
			}
			el = el.CancelTimeout()
			el.MustFocus()
			page.MustInsertText(text)
		case "clickxy":
			var x, y float64
			fmt.Sscanf(arg, "%f,%f", &x, &y)
			page.Mouse.MustMoveTo(x, y)
			page.Mouse.MustClick(proto.InputMouseButtonLeft)
		case "insert":
			page.MustInsertText(arg)
		case "enter":
			page.Keyboard.MustType(input.Enter)
		case "dump":
			r, err := page.Eval(dumpJS, arg)
			if err != nil {
				fmt.Println("dump err", err)
				continue
			}
			fmt.Println("== dump", arg, "url=", page.MustInfo().URL)
			fmt.Println(r.Value.String())
		case "text":
			r, err := page.Eval(`(s) => [...document.querySelectorAll(s)].map(e => e.innerText).join('\n---\n')`, arg)
			if err == nil {
				fmt.Println("== text", arg)
				fmt.Println(r.Value.String())
			}
		case "shot":
			img, _ := page.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
			_ = os.WriteFile(out+"/"+arg+".png", img, 0o644)
		}
	}
}
