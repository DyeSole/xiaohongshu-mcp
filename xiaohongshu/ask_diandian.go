package xiaohongshu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/humanize"
)

// 设计是：问点点复用打开页面那一套，打开点点（/ai_chat），带完整笔记链接提问，
// 等回答说完再读出文字。短链点点不认，调用方要先换成完整链接。
const (
	// 2026-11 点点改版：/ai_chat 直接进上次的会话，没有欢迎页了，输入框换成 .ai-chat-input-box；旧的也认。
	diandianInputSelector  = ".ai-chat-input-box textarea, .ai-chat-welcome__input textarea"
	diandianMessageClass   = ".ai-message"
	diandianAnswerSelector = ".markdown-block"
	diandianFinishedClass  = ".ai-message.ai-message-finished"
	diandianAnswerTimeout  = 2 * time.Minute
	// 设计是：搜索改成问点点找笔记；点开“搜索并总结N人真实经验”，右侧“参考来源”
	// 面板里每张卡片都有笔记 ID、xsec_token、标题、封面，封面由 Yooo7 拼成一张图。
	// 2026-11 改版后按钮是“参考小红书与全网N篇内容”（.xhs-spark-source-header），旧的胶囊也认。
	diandianSourcesButton = ".xhs-spark-source-header, .xhs-v3-tool-call-capsule"
	diandianSourceTitle   = ".xhs-ai-chat-page-right a.title"
	// 2026-11 改版后的参考来源卡片：没有笔记链接了，点卡片才跳到 /explore/ID?xsec_token=…
	diandianSourceCard = ".xhs-ai-chat-page-right .xhs-sq-source-card"
)

// DiandianSource 点点“参考来源”里的一篇笔记。
type DiandianSource struct {
	ID        string `json:"id"`
	XsecToken string `json:"xsec_token"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Likes     string `json:"likes"`
	Cover     string `json:"cover"`
	Video     bool   `json:"video"`
}

type AskDiandianAction struct {
	page *rod.Page
}

func NewAskDiandianAction(page *rod.Page) *AskDiandianAction {
	return &AskDiandianAction{page: page}
}

// Ask 在点点里发"笔记链接 + 问题"，返回点点的回答文字。
func (a *AskDiandianAction) Ask(ctx context.Context, noteURL, question string) (string, error) {
	page := a.page.Context(ctx).Timeout(diandianAnswerTimeout + time.Minute)
	before, err := sendDiandian(ctx, page, noteURL+" "+question)
	if err != nil {
		return "", err
	}
	logrus.Infof("已向点点提问: %s", noteURL)
	return waitDiandianAnswer(ctx, page, before)
}

// FindNotes 让点点找笔记，返回回答文字和“参考来源”里的笔记。
func (a *AskDiandianAction) FindNotes(ctx context.Context, keyword string) (string, []DiandianSource, error) {
	page := a.page.Context(ctx).Timeout(diandianAnswerTimeout + time.Minute)
	// 设计是：你依据小红书帖子帮我查查「关键词」（user 定的原话）。
	before, err := sendDiandian(ctx, page, "你依据小红书帖子帮我查查「"+keyword+"」")
	if err != nil {
		return "", nil, err
	}
	logrus.Infof("已让点点找笔记: %s", keyword)
	answer, err := waitDiandianAnswer(ctx, page, before)
	if err != nil {
		return "", nil, err
	}
	sources, err := readDiandianSources(ctx, page, before)
	if err != nil {
		// 回答本身还有用，来源读不到只记日志。
		logrus.Warnf("读取点点参考来源失败: %v", err)
	}
	return answer, sources, nil
}

func readDiandianSources(ctx context.Context, page *rod.Page, before diandianCounts) ([]DiandianSource, error) {
	// 会话里有以前的参考来源按钮，只点这次回答（最后一条 .ai-message）里的最后一个。
	var button *rod.Element
	for i := 0; i < 10 && button == nil; i++ {
		messages, err := page.Elements(diandianMessageClass)
		if err == nil && len(messages) > before.messages {
			if buttons, err := messages[len(messages)-1].Elements(diandianSourcesButton); err == nil && len(buttons) > 0 {
				button = buttons[len(buttons)-1]
			}
		}
		if button == nil {
			time.Sleep(time.Second)
		}
	}
	if button == nil {
		return nil, fmt.Errorf("没有参考来源按钮")
	}
	if err := button.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("点开参考来源失败: %w", err)
	}
	panel, err := page.Timeout(15 * time.Second).Element(diandianSourceCard + ", " + diandianSourceTitle)
	if err != nil {
		return nil, fmt.Errorf("参考来源面板没出来: %w", err)
	}
	humanize.Delay(ctx, humanize.AfterNavigate)
	if isCard, _ := panel.Matches(diandianSourceCard); isCard {
		return readDiandianSourceCards(page)
	}
	result, err := page.Eval(`(selector) => [...document.querySelectorAll(selector)].map(a => {
		let card = a;
		for (let k = 0; k < 5 && card && !card.querySelector('img:not(.author-avatar)'); k++) card = card.parentElement;
		const cover = card && [...card.querySelectorAll('img')].find(i => !/avatar/.test(i.src));
		const m = (a.getAttribute('href') || '').match(/\/(?:search_result|explore)\/([0-9a-f]{24})\?xsec_token=([^&]+)/);
		const text = el => el ? el.innerText.trim() : '';
		return {
			id: m ? m[1] : '', xsec_token: m ? decodeURIComponent(m[2]) : '',
			title: a.innerText.trim(),
			author: text(card && card.querySelector('.author .name, .author span, .author')),
			likes: text(card && card.querySelector('[class*=like] .count, [class*=like]')),
			cover: cover ? cover.src : '',
			video: !!(card && card.querySelector('[class*=play]')),
		};
	}).filter(note => note.id && note.xsec_token)`, diandianSourceTitle)
	if err != nil {
		return nil, fmt.Errorf("读取参考来源失败: %w", err)
	}
	var sources []DiandianSource
	if err := result.Value.Unmarshal(&sources); err != nil {
		return nil, fmt.Errorf("解析参考来源失败: %w", err)
	}
	return sources, nil
}

// readDiandianSourceCards 新版卡片：拦住 pushState/window.open，挨个点卡片记下要跳的笔记地址，不真跳。
func readDiandianSourceCards(page *rod.Page) ([]DiandianSource, error) {
	result, err := page.Eval(`async (selector) => {
		const picked = [];
		const isNote = url => url && /\/explore\/[0-9a-f]{24}/.test(String(url));
		const keep = fn => function (state, title, url) { if (isNote(url)) { picked.push(String(url)); return; } return fn.apply(this, arguments); };
		const push = history.pushState, replace = history.replaceState, open = window.open;
		history.pushState = keep(push.bind(history));
		history.replaceState = keep(replace.bind(history));
		window.open = function (url) { if (isNote(url)) { picked.push(String(url)); return null; } return open.apply(window, arguments); };
		const text = el => el ? (el.innerText || el.textContent || '').trim() : '';
		const notes = [];
		try {
			for (const card of document.querySelectorAll(selector)) {
				const before = picked.length;
				(card.querySelector('.xhs-sq-source-card__body') || card).click();
				await new Promise(r => setTimeout(r, 400));
				const url = picked.length > before ? picked[picked.length - 1] : '';
				const m = url.match(/\/explore\/([0-9a-f]{24})\?(?:.*&)?xsec_token=([^&]+)/);
				if (!m) continue;
				const likes = [...card.querySelectorAll('*')].map(el => el.childElementCount ? '' : (el.textContent || '').trim()).find(t => /^获赞/.test(t)) || '';
				const cover = card.querySelector('.xhs-sq-source-card__covers img');
				notes.push({
					id: m[1], xsec_token: decodeURIComponent(m[2]),
					title: text(card.querySelector('.xhs-sq-source-card__title')) || text(card.querySelector('.xhs-sq-source-card__content')).slice(0, 40),
					author: text(card.querySelector('.xhs-sq-source-card__name')),
					likes: likes.replace(/^获赞\s*/, ''),
					cover: cover ? cover.src : '',
					video: !!card.querySelector('[class*=play], [class*=video]'),
				});
			}
		} finally {
			history.pushState = push; history.replaceState = replace; window.open = open;
		}
		return notes;
	}`, diandianSourceCard)
	if err != nil {
		return nil, fmt.Errorf("读取参考来源失败: %w", err)
	}
	var sources []DiandianSource
	if err := result.Value.Unmarshal(&sources); err != nil {
		return nil, fmt.Errorf("解析参考来源失败: %w", err)
	}
	return sources, nil
}

// diandianCounts 发问题前页面上已有的回答条数和参考来源按钮个数，用来认出这次新出的回答。
type diandianCounts struct {
	messages int
	sources  int
}

func countDiandian(page *rod.Page) diandianCounts {
	result, err := page.Eval(`(m, s) => [document.querySelectorAll(m).length, document.querySelectorAll(s).length]`,
		diandianMessageClass, diandianSourcesButton)
	if err != nil {
		return diandianCounts{}
	}
	values := result.Value.Arr()
	if len(values) != 2 {
		return diandianCounts{}
	}
	return diandianCounts{messages: values[0].Int(), sources: values[1].Int()}
}

// waitDiandianHistory 点点会进上次的会话，旧消息是陆续加载出来的；条数 2 秒不变才算加载完（最多等 10 秒）。
func waitDiandianHistory(ctx context.Context, page *rod.Page) diandianCounts {
	last, stableSince := countDiandian(page), time.Now()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && ctx.Err() == nil; {
		time.Sleep(500 * time.Millisecond)
		if now := countDiandian(page); now != last {
			last, stableSince = now, time.Now()
		} else if time.Since(stableSince) >= 2*time.Second {
			break
		}
	}
	return last
}

// sendDiandian 打开点点，点一下输入框，把整段文字粘进去并发送；返回发送前的条数。
func sendDiandian(ctx context.Context, page *rod.Page, text string) (diandianCounts, error) {

	if err := page.Navigate(webURL("/ai_chat")); err != nil {
		return diandianCounts{}, fmt.Errorf("打开点点失败: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return diandianCounts{}, fmt.Errorf("等待点点页面加载失败: %w", err)
	}
	humanize.Delay(ctx, humanize.AfterNavigate)

	inputBox, err := page.Timeout(20 * time.Second).Element(diandianInputSelector)
	if err != nil {
		return diandianCounts{}, fmt.Errorf("未找到点点输入框（可能未登录或页面改版）: %w", err)
	}
	before := waitDiandianHistory(ctx, page)
	inputBox = inputBox.CancelTimeout()
	// 输入框要收到一次真实点击才能输入（只聚焦不行）；上面盖着占位提示层，
	// 评论那种带落点检查的点击会一直卡住，所以按坐标点输入框中心。
	shape, err := inputBox.Shape()
	if err != nil || shape.Box() == nil {
		return diandianCounts{}, fmt.Errorf("无法定位点点输入框: %v", err)
	}
	box := shape.Box()
	if err := humanize.ClickAt(page, proto.Point{X: box.X + box.Width/2, Y: box.Y + box.Height/2}); err != nil {
		return diandianCounts{}, fmt.Errorf("无法点击点点输入框: %w", err)
	}
	humanize.Delay(ctx, humanize.AfterClick)

	// 链接和问题一起整段粘贴：输入后输入框会重新渲染，逐字输入会拿着失效的元素一直等。
	if err := page.InsertText(text); err != nil {
		return diandianCounts{}, fmt.Errorf("无法输入问题: %w", err)
	}
	humanize.Delay(ctx, humanize.AfterType)

	if err := page.Keyboard.Type(input.Enter); err != nil {
		return diandianCounts{}, fmt.Errorf("发送问题失败: %w", err)
	}
	return before, nil
}

// waitDiandianAnswer 等这次新出的回答（条数比发送前多）带上 ai-message-finished 标记，再读最后一条回答的文字。
// 会话里有以前答完的回答，不能看到 finished 就读，否则读到的是上一次的旧回答。
func waitDiandianAnswer(ctx context.Context, page *rod.Page, before diandianCounts) (string, error) {
	deadline := time.Now().Add(diandianAnswerTimeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		state, err := page.Eval(`(m, f) => {
			const all = document.querySelectorAll(m);
			return [all.length, all.length > 0 && all[all.length - 1].matches(f)];
		}`, diandianMessageClass, diandianFinishedClass)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		values := state.Value.Arr()
		if len(values) == 2 && values[0].Int() > before.messages && values[1].Bool() {
			// 一条回答里分好几段 markdown-block，读最后一条回答里的全部段落。
			result, err := page.Eval(`(m, s) => {
				const all = document.querySelectorAll(m);
				const last = all[all.length - 1];
				return last ? [...last.querySelectorAll(s)].map(b => b.innerText.trim()).filter(Boolean).join('\n\n') : '';
			}`, diandianMessageClass, diandianAnswerSelector)
			if err != nil {
				return "", fmt.Errorf("读取点点回答失败: %w", err)
			}
			answer := strings.TrimSpace(result.Value.String())
			if answer == "" {
				return "", fmt.Errorf("点点回答为空")
			}
			return answer, nil
		}
		time.Sleep(time.Second)
	}
	return "", fmt.Errorf("等待点点回答超时（%s）", diandianAnswerTimeout)
}
